package main

import (
	"flag" // New import for command-line flags
	"fmt"
	"log"
	"os"

	// Import the parent package
	multitenancy "github.com/nightDevz/gorm-multitenancy/v2"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	// Import lib/pq for the sql.Open call inside provisioner/runner
	_ "github.com/lib/pq"
)

// These variables hold the environment key names and are initialized with defaults.
// They are updated by the 'flag' package during parsing.
var dsnEnvKey string
var migrationsDirEnvKey string

func init() {
	// Define command-line flags that allow overriding the default environment variable names.
	flag.StringVar(&dsnEnvKey, "dsn-env", "DB_DSN", "Environment variable name containing the PostgreSQL DSN.")
	flag.StringVar(&migrationsDirEnvKey, "migrations-env", "MIGRATIONS_DIR", "Environment variable name containing the path to migration files.")
}

func main() {
	// Parse the flags defined in init(). This populates dsnEnvKey and migrationsDirEnvKey.
	flag.Parse()

	// 1. Load configuration from Environment Variables using the flag-determined keys.
	baseDSN := os.Getenv(dsnEnvKey)
	if baseDSN == "" {
		log.Fatalf("❌ Error: DSN environment variable '%s' is required.", dsnEnvKey)
	}

	migrationsDir := os.Getenv(migrationsDirEnvKey)
	if migrationsDir == "" {
		log.Fatalf("❌ Error: Migrations directory environment variable '%s' is required.", migrationsDirEnvKey)
	}

	// 2. Connect to DB
	db, err := gorm.Open(postgres.Open(baseDSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ Failed to connect to database: %v", err)
	}

	// 3. Lock the connection for security
	multitenancy.LockDBConnection(db)

	// 4. Create the migration runner
	runner := multitenancy.NewMigrationRunner(
		db,
		baseDSN,
		migrationsDir,
	)

	// 5. Parse command-line arguments (the actual command: up or down)
	if len(flag.Args()) < 1 {
		printUsage()
		os.Exit(1)
	}

	command := flag.Args()[0] // Get the first non-flag argument (e.g., "up")

	// 6. Run the command
	switch command {
	case "up":
		log.Println("🔄 Running 'up' migrations for all tenants...")
		if err := runner.RunUp(); err != nil {
			log.Fatalf("❌ Migration 'up' failed: %v", err)
		}
	case "down":
		log.Println("🔄 Running 'down' migration for all tenants...")
		if err := runner.RunDown(); err != nil {
			log.Fatalf("❌ Migration 'down' failed: %v", err)
		}
	default:
		log.Printf("❌ Unknown command: %s", command)
		printUsage()
		os.Exit(1)
	}

	log.Println("✅ Migration run complete.")
}

func printUsage() {
	fmt.Println("gmt-migrate - Multi-Tenant Migration Tool")
	fmt.Println("------------------------------------------------")
	fmt.Println("Usage:")
	fmt.Println("  gmt-migrate [flags] up      # Apply pending migrations to all tenants")
	fmt.Println("  gmt-migrate [flags] down    # Rollback last migration for all tenants")
	fmt.Println("\nFlags (Environment Variable Overrides):")
	flag.PrintDefaults()
	fmt.Println("\nExample (Using custom environment variables):")
	fmt.Println("  gmt-migrate -dsn-env=POC_DB_DSN -migrations-env=APP_MIGRATIONS_DIR up")
}
