package multitenancy

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/pressly/goose/v3"
	"gorm.io/gorm"
)

// MigrationRunner handles running migrations across all tenants.
type MigrationRunner struct {
	db            *gorm.DB
	writerDSN     string
	migrationsDir string
}

// NewMigrationRunner creates a new MigrationRunner.
func NewMigrationRunner(db *gorm.DB, writerDSN string, migrationsDir string) *MigrationRunner {
	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatalf("[gorm-multitenancy] ❌ Failed to set goose dialect: %v", err)
	}
	return &MigrationRunner{
		db:            db,
		writerDSN:     writerDSN,
		migrationsDir: migrationsDir,
	}
}

// RunUp applies all pending "up" migrations to every tenant.
func (r *MigrationRunner) RunUp() error {
	log.Println("🚀 Starting 'up' migrations for all tenants...")

	tenants, err := r.getAllTenants()
	if err != nil {
		return err
	}

	for _, tenant := range tenants {
		safeSchemaName, err := SanitizeSchemaName(tenant.SchemaName)
		if err != nil {
			log.Printf("⚠️  SKIPPING tenant %s: invalid name in registry: %v", tenant.SchemaName, err)
			continue
		}

		log.Printf("--- Migrating tenant: %s ---", safeSchemaName)

		tenantDB, err := r.getTenantDB(safeSchemaName)
		if err != nil {
			log.Printf("⚠️  SKIPPING tenant %s: failed to create DB connection: %v", safeSchemaName, err)
			continue
		}

		if err := goose.Up(tenantDB, r.migrationsDir); err != nil {
			log.Printf("❌ FAILED migration for tenant %s: %v", safeSchemaName, err)
		} else {
			log.Printf("✅ Successfully migrated tenant: %s", safeSchemaName)
		}

		tenantDB.Close()
	}

	log.Println("--- All tenant 'up' migrations complete. ---")
	return nil
}

// RunDown rolls back the single most recent migration for every tenant.
func (r *MigrationRunner) RunDown() error {
	log.Println("⏪ Starting 'down' migrations (rollback) for all tenants...")

	tenants, err := r.getAllTenants()
	if err != nil {
		return err
	}

	for _, tenant := range tenants {
		safeSchemaName, err := SanitizeSchemaName(tenant.SchemaName)
		if err != nil {
			continue
		}

		log.Printf("--- Rolling back tenant: %s ---", safeSchemaName)

		tenantDB, err := r.getTenantDB(safeSchemaName)
		if err != nil {
			log.Printf("⚠️  SKIPPING tenant %s: failed to connect: %v", safeSchemaName, err)
			continue
		}

		if err := goose.Down(tenantDB, r.migrationsDir); err != nil {
			log.Printf("❌ FAILED rollback for tenant %s: %v", safeSchemaName, err)
		} else {
			log.Printf("✅ Successfully rolled back tenant: %s", safeSchemaName)
		}

		tenantDB.Close()
	}

	log.Println("--- All tenant 'down' migrations complete. ---")
	return nil
}

// getTenantDB creates a new *sql.DB pool scoped to a specific tenant.
func (r *MigrationRunner) getTenantDB(safeSchemaName string) (*sql.DB, error) {
	// DDL requires the Writer DSN
	tenantDSN := fmt.Sprintf("%s search_path=%s,public", r.writerDSN, safeSchemaName)

	tenantDB, err := sql.Open("postgres", tenantDSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open tenant-scoped DB connection: %w", err)
	}

	if err := tenantDB.Ping(); err != nil {
		tenantDB.Close()
		return nil, fmt.Errorf("failed to ping tenant-scoped DB: %w", err)
	}
	return tenantDB, nil
}

// getAllTenants fetches the list of schemas from the registry.
func (r *MigrationRunner) getAllTenants() ([]PublicTenant, error) {
	var tenants []PublicTenant
	if err := r.db.Find(&tenants).Error; err != nil {
		return nil, fmt.Errorf("failed to get tenant list: %w", err)
	}
	if len(tenants) == 0 {
		log.Println("No tenants found in public.tenants table.")
	} else {
		log.Printf("Found %d tenants to migrate.", len(tenants))
	}
	return tenants, nil
}
