package main

import (
	"fmt"
	"log"
	"os"

	// Import the parent package
	multitenancy "github.com/nightDevz/gorm-multitenancy"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	// Import lib/pq for the sql.Open call inside provisioner/runner
	_ "github.com/lib/pq"
)

func main() {
	// 1. Load configuration from Environment Variables
	baseDSN := os.Getenv("DB_DSN")
	if baseDSN == "" {
		log.Fatal("❌ Error: DB_DSN environment variable is required. Example: export DB_DSN='host=... user=...'")
	}

	migrationsDir := os.Getenv("MIGRATIONS_DIR")
	if migrationsDir == "" {
		log.Fatal("❌ Error: MIGRATIONS_DIR environment variable is required. Example: export MIGRATIONS_DIR='./migrations'")
	}

	// 2. Connect to DB
	db, err := gorm.Open(postgres.Open(baseDSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ Failed to connect to database: %v", err)
	}

	// 3. Lock the connection for security
	// This also runs the GORM plugin's AutoMigrate indirectly via the GORM setup.
	multitenancy.LockDBConnection(db)

	// 4. Create the migration runner
	runner := multitenancy.NewMigrationRunner(
		db,
		baseDSN,
		migrationsDir,
	)

	// 5. Parse command-line arguments
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

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
	fmt.Println("  gmt-migrate up      # Apply pending migrations to all tenants")
	fmt.Println("  gmt-migrate down    # Rollback last migration for all tenants")
	fmt.Println("\nEnvironment Variables required:")
	fmt.Println("  DB_DSN             (e.g., host=localhost user=postgres ...)")
	fmt.Println("  MIGRATIONS_DIR     (e.g., ./migrations)")
}
