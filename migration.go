package multitenancy

import (
	"database/sql" // Import database/sql
	"fmt"
	"log"

	"github.com/pressly/goose/v3"
	"gorm.io/gorm"
	// Import your postgres driver
	// e.g., _ "github.com/lib/pq"
	// or _ "github.com/jackc/pgx/v5/stdlib"
)

// Tenant model for reading from the public registry
type Tenant struct {
	SchemaName string `gorm:"column:schema_name"`
}

// TableName explicitly sets the table to the public schema
func (Tenant) TableName() string {
	return "public.tenants"
}

// MigrationRunner handles running migrations across all tenants.
type MigrationRunner struct {
	db            *gorm.DB // Master connection, for getting tenant list
	baseDSN       string   // Base DSN to create new connections
	migrationsDir string
}

// NewMigrationRunner creates a new MigrationRunner.
func NewMigrationRunner(db *gorm.DB, baseDSN string, migrationsDir string) *MigrationRunner {
	if err := goose.SetDialect("postgres"); err != nil { // Or your dialect
		log.Fatalf("❌ Failed to set goose dialect: %v", err)
	}
	return &MigrationRunner{
		db:            db,
		baseDSN:       baseDSN,
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
			log.Printf("⚠️ SKIPPING tenant %s: invalid name in registry: %v", tenant.SchemaName, err)
			continue
		}

		log.Printf("--- Migrating tenant: %s ---", safeSchemaName)

		// Create a new, scoped connection *for each tenant*
		tenantDB, err := r.getTenantDB(safeSchemaName)
		if err != nil {
			log.Printf("⚠️ SKIPPING tenant %s: failed to create DB connection: %v", safeSchemaName, err)
			continue
		}

		// Run goose.Up on the scoped *sql.DB
		if err := goose.Up(tenantDB, r.migrationsDir); err != nil {
			log.Printf("❌ FAILED migration for tenant %s: %v", safeSchemaName, err)
		} else {
			log.Printf("✅ Successfully migrated tenant: %s", safeSchemaName)
		}

		tenantDB.Close() // Close the pool for this tenant
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
			log.Printf("⚠️ SKIPPING tenant %s: invalid name in registry: %v", tenant.SchemaName, err)
			continue
		}

		log.Printf("--- Rolling back tenant: %s ---", safeSchemaName)

		tenantDB, err := r.getTenantDB(safeSchemaName)
		if err != nil {
			log.Printf("⚠️ SKIPPING tenant %s: failed to create DB connection: %v", safeSchemaName, err)
			continue
		}

		// Run goose.Down on the scoped *sql.DB
		if err := goose.Down(tenantDB, r.migrationsDir); err != nil {
			log.Printf("❌ FAILED rollback for tenant %s: %v", safeSchemaName, err)
		} else {
			log.Printf("✅ Successfully rolled back tenant: %s", safeSchemaName)
		}

		tenantDB.Close() // Close the pool
	}

	log.Println("--- All tenant 'down' migrations complete. ---")
	return nil
}

// getTenantDB creates a new *sql.DB pool scoped to a specific tenant.
func (r *MigrationRunner) getTenantDB(safeSchemaName string) (*sql.DB, error) {
	// Note: DSN format varies. This " " space separator works for lib/pq.
	tenantDSN := fmt.Sprintf("%s search_path=%s,public", r.baseDSN, safeSchemaName)

	tenantDB, err := sql.Open("postgres", tenantDSN) // Use your driver name
	if err != nil {
		return nil, fmt.Errorf("failed to open tenant-scoped DB connection: %w", err)
	}

	if err := tenantDB.Ping(); err != nil {
		tenantDB.Close()
		return nil, fmt.Errorf("failed to ping tenant-scoped DB: %w", err)
	}
	return tenantDB, nil
}

// getAllTenants fetches the list of schemas from the registry (uses master db).
func (r *MigrationRunner) getAllTenants() ([]Tenant, error) {
	var tenants []Tenant
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
