package multitenancy

import (
	"context"
	"database/sql" // Import database/sql
	"fmt"
	"log"

	"github.com/pressly/goose/v3"
	"gorm.io/gorm"
	// Import your postgres driver to be able to sql.Open
	// e.g., _ "github.com/lib/pq"
	// or _ "github.com/jackc/pgx/v5/stdlib"
)

// TenantProvisioner handles provisioning new tenants.
type TenantProvisioner struct {
	db            *gorm.DB // The master connection
	baseDSN       string   // The base DSN, e.g., "host=... user=... dbname=..."
	migrationsDir string
}

// NewTenantProvisioner creates a new TenantProvisioner.
// 'baseDSN' is your master connection string, without search_path.
func NewTenantProvisioner(db *gorm.DB, baseDSN string, migrationsDir string) *TenantProvisioner {
	if err := goose.SetDialect("postgres"); err != nil { // Or your dialect
		log.Fatalf("❌ Failed to set goose dialect: %v", err)
	}
	return &TenantProvisioner{
		db:            db,
		baseDSN:       baseDSN,
		migrationsDir: migrationsDir,
	}
}

// ProvisionTenant creates a schema, registers it, and migrates it.
func (r *TenantProvisioner) ProvisionTenant(ctx context.Context, schemaName string) (err error) {
	// 1. Sanitize the name.
	safeSchemaName, err := SanitizeSchemaName(schemaName)
	if err != nil {
		return err
	}

	// 2. Run schema creation and tenant registration in a single transaction
	//    This uses the master 'r.db' connection.
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Use fmt.Sprintf for DDL - safe due to sanitizer
		createSchemaQuery := fmt.Sprintf("CREATE SCHEMA %s", safeSchemaName)
		if err := tx.Exec(createSchemaQuery).Error; err != nil {
			return fmt.Errorf("failed to create schema: %w", err)
		}

		// Uses raw SQL for consistency, inserting into the auto-migrated public.tenants table
		registerTenantQuery := "INSERT INTO public.tenants (schema_name) VALUES (?)"
		if err := tx.Exec(registerTenantQuery, safeSchemaName).Error; err != nil {
			return fmt.Errorf("failed to register tenant: %w", err)
		}
		return nil
	})

	if err != nil {
		return err // Transaction failed, do not proceed
	}

	// 3. If transaction succeeded, run migrations using a new, scoped DB pool.
	log.Printf("Tenant %s registered. Running migrations...", safeSchemaName)

	// Create a new DSN scoped to the tenant's schema
	tenantDSN := fmt.Sprintf("%s search_path=%s,public", r.baseDSN, safeSchemaName)

	tenantDB, err := sql.Open("postgres", tenantDSN) // Use your driver name
	if err != nil {
		return fmt.Errorf("failed to open tenant-scoped DB connection: %w", err)
	}
	defer tenantDB.Close()

	if err := tenantDB.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping tenant-scoped DB: %w", err)
	}

	// 4. Run goose.Up on the new, tenant-scoped *sql.DB
	if err = goose.Up(tenantDB, r.migrationsDir); err != nil {
		// If migration fails, we must attempt to roll back
		log.Printf("❌ FAILED to migrate new tenant %s: %v. Rolling back...", safeSchemaName, err)

		// This is a "best effort" rollback
		dropSchemaQuery := fmt.Sprintf("DROP SCHEMA %s CASCADE", safeSchemaName)
		if dropErr := r.db.Exec(dropSchemaQuery).Error; dropErr != nil {
			log.Printf("!! CRITICAL: FAILED to drop schema %s: %v", safeSchemaName, dropErr)
		}

		deleteTenantQuery := "DELETE FROM public.tenants WHERE schema_name = ?"
		if delErr := r.db.Exec(deleteTenantQuery, safeSchemaName).Error; delErr != nil {
			log.Printf("!! CRITICAL: FAILED to delete tenant %s from registry: %v", safeSchemaName, delErr)
		}

		return fmt.Errorf("failed to run goose migrations for new tenant: %w", err)
	}

	log.Printf("✅ Successfully provisioned and migrated new tenant: %s", safeSchemaName)
	return nil
}
