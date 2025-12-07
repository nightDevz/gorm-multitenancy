package multitenancy

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/pressly/goose/v3"
	"gorm.io/gorm"
)

// TenantProvisioner handles provisioning new tenants.
type TenantProvisioner struct {
	db            *gorm.DB // The master connection (can be a pool/resolver)
	writerDSN     string   // Connection string for the Writer instance (required for DDL)
	migrationsDir string
}

// NewTenantProvisioner creates a new TenantProvisioner.
func NewTenantProvisioner(db *gorm.DB, writerDSN string, migrationsDir string) *TenantProvisioner {
	// Set dialect once during initialization
	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatalf("[gorm-multitenancy] ❌ Failed to set goose dialect: %v", err)
	}
	return &TenantProvisioner{
		db:            db,
		writerDSN:     writerDSN,
		migrationsDir: migrationsDir,
	}
}

// ProvisionTenant creates a schema, registers it, and migrates it.
func (r *TenantProvisioner) ProvisionTenant(ctx context.Context, schemaName string) (err error) {
	safeSchemaName, err := SanitizeSchemaName(schemaName)
	if err != nil {
		return err
	}

	// 1. Pre-flight Check: Ensure tenant does not already exist.
	//    We check both the registry table and the Postgres information schema.
	var existsCount int64

	// Check Registry
	if err := r.db.Model(&PublicTenant{}).Where("schema_name = ?", safeSchemaName).Count(&existsCount).Error; err != nil {
		return fmt.Errorf("failed to check tenant registry: %w", err)
	}
	if existsCount > 0 {
		return fmt.Errorf("tenant '%s' already exists in registry", safeSchemaName)
	}

	// Check Postgres Information Schema (detects orphan schemas)
	checkSchemaSQL := "SELECT count(*) FROM information_schema.schemata WHERE schema_name = ?"
	if err := r.db.Raw(checkSchemaSQL, safeSchemaName).Scan(&existsCount).Error; err != nil {
		return fmt.Errorf("failed to verify schema existence: %w", err)
	}
	if existsCount > 0 {
		return fmt.Errorf("schema '%s' already exists in database (orphan)", safeSchemaName)
	}

	// 2. Create Schema & Register (Atomic Transaction)
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// DDL: Create Schema
		createSchemaQuery := fmt.Sprintf("CREATE SCHEMA %s", safeSchemaName)
		if err := tx.Exec(createSchemaQuery).Error; err != nil {
			return fmt.Errorf("failed to create schema: %w", err)
		}

		// DML: Register Tenant
		registerTenantQuery := "INSERT INTO public.tenants (schema_name) VALUES (?)"
		if err := tx.Exec(registerTenantQuery, safeSchemaName).Error; err != nil {
			return fmt.Errorf("failed to register tenant: %w", err)
		}
		return nil
	})

	if err != nil {
		return err // Transaction failed/rolled back. No data loss.
	}

	// 3. Run Migrations
	//    If failure occurs here, we must rollback the schema creation from Step 2.
	log.Printf("[gorm-multitenancy] Tenant %s registered. Running migrations...", safeSchemaName)

	if err := r.runMigrationsForNewTenant(ctx, safeSchemaName); err != nil {
		log.Printf("[gorm-multitenancy] ❌ FAILED to migrate new tenant %s: %v. Initiating rollback...", safeSchemaName, err)

		// Rollback: Drop the specific schema and registry entry.
		_ = r.db.Exec(fmt.Sprintf("DROP SCHEMA %s CASCADE", safeSchemaName))
		_ = r.db.Exec("DELETE FROM public.tenants WHERE schema_name = ?", safeSchemaName)

		return fmt.Errorf("provisioning failed during migration: %w", err)
	}

	log.Printf("[gorm-multitenancy] ✅ Successfully provisioned and migrated new tenant: %s", safeSchemaName)
	return nil
}

// runMigrationsForNewTenant handles the specific connection logic for running goose.
func (r *TenantProvisioner) runMigrationsForNewTenant(ctx context.Context, safeSchemaName string) error {
	// Construct a DSN specifically for this tenant's search_path
	tenantDSN := fmt.Sprintf("%s search_path=%s,public", r.writerDSN, safeSchemaName)

	tenantDB, err := sql.Open("postgres", tenantDSN)
	if err != nil {
		return fmt.Errorf("failed to open tenant-scoped DB connection: %w", err)
	}
	defer tenantDB.Close()

	if err := tenantDB.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping tenant-scoped DB: %w", err)
	}

	// Run migrations using Goose
	return goose.Up(tenantDB, r.migrationsDir)
}
