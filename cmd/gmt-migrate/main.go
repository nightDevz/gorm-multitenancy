package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	multitenancy "github.com/nightDevz/gorm-multitenancy/v2"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	_ "github.com/lib/pq"
)

var (
	dsnEnvKey           string
	migrationsDirEnvKey string
	forceFlag           bool
)

func init() {
	flag.StringVar(&dsnEnvKey, "dsn-env", "DB_DSN", "Environment variable name containing the PostgreSQL WRITER DSN.")
	flag.StringVar(&migrationsDirEnvKey, "migrations-env", "MIGRATIONS_DIR", "Environment variable name containing the path to migration files.")
	flag.BoolVar(&forceFlag, "force", false, "Skip confirmation prompts (Dangerous). Use for CI/CD automation.")
}

func main() {
	flag.Parse()

	// 1. Load configuration
	writerDSN := os.Getenv(dsnEnvKey)
	if writerDSN == "" {
		log.Fatalf("❌ Error: WRITER DSN environment variable '%s' is required.", dsnEnvKey)
	}

	migrationsDir := os.Getenv(migrationsDirEnvKey)
	if migrationsDir == "" {
		log.Fatalf("❌ Error: Migrations directory environment variable '%s' is required.", migrationsDirEnvKey)
	}

	// 2. Connect to DB (Used for reading tenant list)
	db, err := gorm.Open(postgres.Open(writerDSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ Failed to connect to database: %v", err)
	}

	// 3. Lock the connection for security
	multitenancy.LockDBConnection(db)

	// 4. Create the migration runner
	runner := multitenancy.NewMigrationRunner(
		db,
		writerDSN, // Explicitly pass the Writer DSN
		migrationsDir,
	)

	// 5. Parse command
	if len(flag.Args()) < 1 {
		printUsage()
		os.Exit(1)
	}

	command := flag.Args()[0]

	switch command {
	case "up":
		log.Println("🔄 Running 'up' migrations for all tenants...")
		if err := runner.RunUp(); err != nil {
			log.Fatalf("❌ Migration 'up' failed: %v", err)
		}
	case "down":
		// SAFETY CHECK: Prompt user before destroying data, unless forced via flag
		if !forceFlag {
			warnUserAndConfirm()
		}

		log.Println("⚠️  Running 'down' migration for all tenants...")
		if err := runner.RunDown(); err != nil {
			log.Fatalf("❌ Migration 'down' failed: %v", err)
		}
	default:
		log.Printf("❌ Unknown command: %s", command)
		printUsage()
		os.Exit(1)
	}

	log.Println("✅ Run complete.")
}

func warnUserAndConfirm() {
	fmt.Println("\n⚠️  DANGER: You are about to run a 'DOWN' migration on ALL tenants.")
	fmt.Println("This can result in IRREVERSIBLE DATA LOSS (dropping tables/columns).")
	fmt.Print("Are you sure you want to proceed? (type 'yes' to confirm): ")

	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(response)

	if strings.ToLower(response) != "yes" {
		fmt.Println("❌ Aborted by user.")
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("gmt-migrate - Multi-Tenant Migration Tool")
	fmt.Println("------------------------------------------------")
	fmt.Println("Usage:")
	fmt.Println("  gmt-migrate [flags] up      # Apply pending migrations to all tenants")
	fmt.Println("  gmt-migrate [flags] down    # Rollback last migration for all tenants")
	fmt.Println("\nFlags:")
	flag.PrintDefaults()
	fmt.Println("\nNote: The DSN provided MUST be the WRITER endpoint (for creating tables).")
}
