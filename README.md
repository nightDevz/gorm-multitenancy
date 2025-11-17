# gorm-multitenancy
# Latest stable v1.0.3


-----

# gorm-multitenancy

Latest stable v1.0.4

A complete solution for building multi-tenant applications with GORM using the **Schema-per-Tenant** strategy. This package provides automatic, secure, and request-scoped tenant isolation by manipulating PostgreSQL's `search_path`.

This package is designed for Go web services (using Gin) and provides utilities for provisioning and migrating tenant schemas.

## ✨ Features

  * **GORM Plugin:** Automatically sets `search_path` for all GORM queries (Create, Query, Update, Delete, Raw, etc.).
  * **Gin Middleware:** Extracts a tenant ID from the `X-Tenant-ID` header, sanitizes it, and injects it into the request `context`.
  * **Secure Provisioning:** A `TenantProvisioner` to safely create new tenant schemas and run initial migrations.
  * **Multi-Tenant Migrations:** A `MigrationRunner` utility to roll out (or roll back) schema changes across all existing tenants at once.
  * **Security First:** Includes a function to "lock" the master connection to a non-existent schema, preventing accidental cross-tenant queries.

## 🏛️ Architecture Overview

This package relies on PostgreSQL schemas. Each tenant (e.g., "acme", "globex") gets their own isolated schema (e.g., `acme`, `globex`).

1.  A central table, **`public.tenants`**, stores a list of all tenant `schema_name`s.
2.  An HTTP request arrives with a header: `X-Tenant-ID: acme`.
3.  The Gin `TenantMiddleware` validates this ID and injects the schema name `"acme"` into the `c.Request.Context()`.
4.  You call `db.WithContext(c.Request.Context()).Find(&products)`.
5.  The GORM `Plugin` intercepts this query, reads `"acme"` from the context, and executes `SET search_path TO acme, public` for that specific database connection or transaction.
6.  GORM's query for `products` is automatically scoped to the `acme.products` table. The `public` schema is included so GORM can find the `public.tenants` table if needed.

## 📦 Installation

This guide assumes your package is available locally (e.g., `your/project/gorm-multitenancy`). You will also need:

```bash
# Install the plugin
go get github.com/nightDevz/gorm-multitenancy

# GORM and its Postgres driver
go get gorm.io/gorm
go get gorm.io/driver/postgres

# Gin for the web framework
go get github.com/gin-gonic/gin

# Goose for SQL migrations
go get github.com/pressly/goose/v3

# A Postgres driver for sql.DB (needed by goose)
go get github.com/lib/pq
# or
go get github.com/jackc/pgx/v5/stdlib
```

-----

## 🚀 Full Usage Guide

Here is a complete guide to setting up a new service.

### Step 1: Database Setup

First, you **must** create the central tenant registry in your `public` schema. The `MigrationRunner` and `TenantProvisioner` depend on this table.

```sql
-- This table lives in the 'public' schema
CREATE TABLE public.tenants (
    id SERIAL PRIMARY KEY,
    schema_name VARCHAR(63) NOT NULL UNIQUE,
    created_at TIMESTZ NOT NULL DEFAULT NOW()
);
```

### Step 2: Create Your Tenant Migrations

Create a `migrations` directory. This package uses `pressly/goose` to manage SQL-first migrations.

**`migrations/001_create_products.sql`**

```sql
-- +goose Up
CREATE TABLE products (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TIMESTZ DEFAULT NOW()
);

-- +goose Down
DROP TABLE products;
```

**`migrations/002_add_sku_to_products.sql`**

```sql
-- +goose Up
ALTER TABLE products ADD COLUMN sku VARCHAR(255);

-- +goose Down
ALTER TABLE products DROP COLUMN sku;
```

### Step 3: Setup Your Service (`main.go`)

This file initializes your GORM plugin and Gin server. It exposes a public "provision" endpoint and a tenant-scoped "products" endpoint.

```go
package main

import (
	"context"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/driver/postgres"

	// Import your package
	"github.com/nightDevz/gorm-multitenancy"

	// Import the SQL driver for the provisioner
	_ "github.com/lib/pq"
)

// Your DSN must be stored securely (e.g., env vars)
const (
	// DSN for GORM (and the provisioner)
	// Does not need a search_path
	BASE_DSN = "host=localhost user=admin password=pass dbname=multi_tenant_db port=5432 sslmode=disable"
	
	// Path to your migration files
	MIGRATIONS_DIR = "./migrations"
)

// This is your tenant-scoped GORM model
type Product struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	SKU  string `json:"sku,omitempty"`
}

// We keep the provisioner global for this example
var provisioner *multitenancy.TenantProvisioner

func main() {
	// 1. Connect to the master database
	db, err := gorm.Open(postgres.Open(BASE_DSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ Failed to connect to database: %v", err)
	}

	// 2. IMPORTANT: Lock the master connection
	multitenancy.LockDBConnection(db)

	// 3. Register the GORM multitenancy plugin
	plugin := multitenancy.NewPlugin(multitenancy.Config{
		TenantKey: multitenancy.TenantSchemaKey,    // from tenant.go
		Sanitizer: multitenancy.SanitizeSchemaName, // from tenant.go
	})

	if err := db.Use(plugin); err != nil {
		log.Fatalf("❌ Failed to register plugin: %v", err)
	}

	// 4. Initialize the Provisioner
	// We pass the master DB, the base DSN (for new connections), and dir
	provisioner = multitenancy.NewTenantProvisioner(db, BASE_DSN, MIGRATIONS_DIR)

	// 5. Setup Gin Router
	r := gin.Default()

	// Public routes (no tenant ID required)
	r.POST("/provision/:tenant_name", handleProvisionTenant)

	// Tenant-scoped routes
	// This middleware secures all routes inside the group
	tenantGroup := r.Group("/api", multitenancy.TenantMiddleware())
	{
		tenantGroup.GET("/products", handleGetProducts(db))
		tenantGroup.POST("/products", handleCreateProduct(db))
	}

	log.Println("🚀 Server starting on :8080")
	r.Run(":8080")
}

// --- Tenant-Scoped Handlers ---
// These handlers run *after* the TenantMiddleware

func handleGetProducts(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var products []Product
		
		// The GORM plugin automatically scopes this query
		// using the tenant from c.Request.Context()
		if err := db.WithContext(c.Request.Context()).Find(&products).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, products)
	}
}

func handleCreateProduct(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var p Product
		if err := c.ShouldBindJSON(&p); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		
		// Also automatically scoped
		if err := db.WithContext(c.Request.Context()).Create(&p).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, p)
	}
}

// --- Public Handler ---
// This handler does NOT use the middleware

func handleProvisionTenant(c *gin.Context) {
	tenantName := c.Param("tenant_name")
	if tenantName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant name required"})
		return
	}

	// We use the app's root context, not the request context
	err := provisioner.ProvisionTenant(context.Background(), tenantName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	c.JSON(http.StatusCreated, gin.H{"status": "tenant created", "tenant": tenantName})
}
```

### Step 4: Creating a Migration Tool (`cmd/migrate/main.go`)

This is a **separate admin tool** you run locally or in your CI/CD pipeline to roll out schema changes to **all existing tenants**.

```go
package main

import (
	"log"
	"os"

	"gorm.io/gorm"
	"gorm.io/driver/postgres"

	// Import your package
	"github.com/nightDevz/gorm-multitenancy"
	
	// Import the SQL driver
	_ "github.com/lib/pq"
)

const (
	BASE_DSN       = "host=localhost user=admin password=pass dbname=multi_tenant_db port=5432 sslmode=disable"
	MIGRATIONS_DIR = "./migrations"
)

func main() {
	// 1. Connect to DB (master connection)
	db, err := gorm.Open(postgres.Open(BASE_DSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ Failed to connect to database: %v", err)
	}
	
	// 2. Lock the connection
	multitenancy.LockDBConnection(db)

	// 3. Create the runner
	runner := multitenancy.NewMigrationRunner(db, BASE_DSN, MIGRATIONS_DIR)

	if len(os.Args) < 2 {
		log.Fatal("Expected 'up' or 'down' command")
	}
	
	command := os.Args[1]

	switch command {
	case "up":
		log.Println("Running 'up' migrations for all tenants...")
		if err := runner.RunUp(); err != nil {
			log.Fatalf("Migration 'up' failed: %v", err)
		}
	case "down":
		log.Println("Running 'down' migration for all tenants...")
		if err := runner.RunDown(); err != nil {
			log.Fatalf("Migration 'down' failed: %v", err)
		}
	default:
		log.Fatalf("Unknown command: %s. Use 'up' or 'down'.", command)
	}
	
	log.Println("✅ Migration run complete.")
}
```

**How to run the migration tool:**

```bash
# Run all pending migrations (e.g., 002_add_sku.sql) on ALL tenants
$ go run ./cmd/migrate/main.go up
Running 'up' migrations for all tenants...
Found 2 tenants to migrate.
--- Migrating tenant: acme ---
✅ Successfully migrated tenant: acme
--- Migrating tenant: globex ---
✅ Successfully migrated tenant: globex
--- All tenant 'up' migrations complete. ---
✅ Migration run complete.

# Roll back one migration on ALL tenants
$ go run ./cmd/migrate/main.go down
Running 'down' migration for all tenants...
...
```

-----

## 🔑 API Reference

### `multitenancy.NewPlugin(config Config)`

Creates the GORM plugin. `Config` requires:

  * `TenantKey any`: The context key to read the schema name from (e.g., `multitenancy.TenantSchemaKey`).
  * `Sanitizer func(string) (string, error)`: The function to validate schema names (e.g., `multitenancy.SanitizeSchemaName`).

### `multitenancy.TenantMiddleware()`

Creates a `gin.HandlerFunc` middleware. It reads the `X-Tenant-ID` header, sanitizes it, and sets it on the `c.Request.Context()` using `TenantSchemaKey`.

### `multitenancy.NewTenantProvisioner(db, baseDSN, migrationsDir)`

Creates a new tenant provisioner.

  * `db *gorm.DB`: The master GORM connection.
  * `baseDSN string`: The raw connection string (no `search_path`) used to create new, short-lived, tenant-scoped connections for `goose`.
  * `migrationsDir string`: Path to your SQL migration files.

### `provisioner.ProvisionTenant(ctx, schemaName)`

Executes the provisioning logic:

1.  Sanitizes the `schemaName`.
2.  Starts a transaction on the master DB.
3.  Creates the new schema (e.g., `CREATE SCHEMA acme`).
4.  Registers the tenant in `public.tenants`.
5.  Commits the transaction.
6.  If successful, creates a *new* tenant-scoped `*sql.DB` connection and runs `goose.Up` to migrate the new schema to the latest version.

### `multitenancy.NewMigrationRunner(db, baseDSN, migrationsDir)`

Creates a new multi-tenant migration runner. Parameters are the same as the provisioner.

### `runner.RunUp()`

Fetches all tenants from `public.tenants`. For each tenant, it creates a new tenant-scoped `*sql.DB` and runs `goose.Up` on it.

### `runner.RunDown()`

Same as `RunUp()`, but runs `goose.Down` to roll back the most recent migration.

### `multitenancy.LockDBConnection(db)`

Executes `SET search_path TO non_existent_schema_lock` on your master GORM `*gorm.DB`. This is a crucial security step to prevent any queries from running on an unsecured (e.g., `public`) connection.

## ⚖️ License

Licensed under the **Apache License, Version 2.0**.