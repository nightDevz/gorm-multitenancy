# gorm-multitenancy

Latest stable v2.0.4

A complete solution for building multi-tenant applications with GORM using the **Schema-per-Tenant** strategy. This package provides automatic, secure, and request-scoped tenant isolation by manipulating PostgreSQL's `search_path`.

This package is designed for Go web services (using Gin) and provides utilities for provisioning and migrating tenant schemas.

-----

## ✨ Features

  * **Auto-Initialized Registry:** Automatically creates the central **`public.tenants`** registry table when the GORM plugin is initialized. Manual SQL setup is no longer required.
  * **Built-in CLI Migration Tool:** Provides the `gmt-migrate` binary, which supports command-line flags to override environment variable names for **microservice flexibility**.
  * **GORM Plugin:** Automatically sets `search_path` for all GORM queries.
  * **Gin Middleware:** Extracts the `X-Tenant-ID` header and injects the schema name into the request `context`.
  * **Security First:** Includes a function to **`LockDBConnection`** to a non-existent schema, preventing accidental cross-tenant queries on the master connection.

-----

## 🏛️ Architecture Overview

This package relies on PostgreSQL schemas. Each tenant (e.g., "acme", "globex") gets their own isolated schema (e.g., `acme`, `globex`).

1.  **Plugin Initialization:** The `db.Use(plugin)` call automatically creates the necessary `public.tenants` registry table.
2.  **Runtime Isolation:** An HTTP request carrying the `X-Tenant-ID` header triggers the **`TenantMiddleware`**.
3.  The **GORM Plugin** then intercepts the query and runs `SET search_path TO <tenant_schema>, public` for that specific database transaction, ensuring perfect data isolation.

-----

## 📦 Installation

To use this solution, you install the dependencies and the **`gmt-migrate`** CLI tool.

```bash
# 1. Install the plugin library using the required /v2 module path
go get github.com/nightDevz/gorm-multitenancy/v2@v2.0.4

# 2. Install the necessary dependencies (if not already installed)
go get gorm.io/gorm github.com/gin-gonic/gin github.com/pressly/goose/v3
go get gorm.io/driver/postgres github.com/lib/pq

# 3. Install the built-in CLI migration tool (gmt-migrate)
go install github.com/nightDevz/gorm-multitenancy/v2/cmd/gmt-migrate@v2.0.4
```

-----

## 🚀 Full Usage Guide

### Step 1: Database and Configuration Setup

1.  **Create the Database:** Log into `psql` and create the master database (e.g., `multitenancy_poc`).

    ```sql
    CREATE DATABASE multitenancy_poc;
    ```

2.  **Configuration:** Define your environment variables in your local shell profile (`~/.zprofile`) or in a secure secrets manager.

### Step 2: Create Your Tenant Migrations

Create a `migrations` directory and add your SQL migration files.

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

### Step 3: Running the Web Service (`main.go`)

This process registers the GORM plugin. The first time this code runs, the plugin's `Initialize` method will automatically create the `public.tenants` table.

```go
package main

import (
    // ... imports, configuration loading ...
    multitenancy "github.com/nightDevz/gorm-multitenancy/v2"
)

// ... (Constants and Product struct) ...

func main() {
    // 1. Connect to the master database
    db := config.InitDB(BASE_DSN) 

    // 2. Register the GORM multitenancy plugin. 
    //    THIS STEP AUTOMATICALLY CREATES the public.tenants table.
    plugin := multitenancy.NewPlugin(multitenancy.Config{
        TenantKey: multitenancy.TenantSchemaKey,
        Sanitizer: multitenancy.SanitizeSchemaName,
    })
    if err := db.Use(plugin); err != nil {
        log.Fatalf("❌ Failed to register plugin: %v", err)
    }
    
    // ... (rest of Dependency Injection, Routing, and Server start) ...
}
```

### Step 4: Running Migrations (The Flexible `gmt-migrate` CLI)

#### 1\. Default Execution (Local Development)

If you use the default environment variable names (`DB_DSN` and `MIGRATIONS_DIR`):

```bash
export DB_DSN="host=localhost..." 
export MIGRATIONS_DIR="./migrations"

gmt-migrate up
```

#### 2\. Microservice Execution (Custom ENV Names)

If your service (e.g., a "Billing Service") uses unique, secure environment variables (e.g., `BILLING_DB_DSN`), you use the flags to tell the tool which variable to read:

```bash
# Set your unique secret variable
export BILLING_DB_DSN="host=db.billing.com..." 

# Run the tool, instructing it to read the custom variable name
gmt-migrate -dsn-env=BILLING_DB_DSN up
```

-----

## 🛠️ Database Migrations & Schema Changes

### Workflow for Adding a New Column

1.  **Update Go Model:** Modify your GORM model (e.g., `Product` struct) to add the new field.

2.  **Create New SQL Script:** Create the sequentially numbered file (e.g., `migrations/003_add_address.sql`) with the reversible DDL:

    ```sql
    -- +goose Up
    ALTER TABLE products ADD COLUMN address TEXT;

    -- +goose Down
    ALTER TABLE products DROP COLUMN address;
    ```

3.  **Execute the Change:** Run the CLI tool:

    ```bash
    gmt-migrate up
    ```

    This applies the `003` migration to all existing tenants (e.g., acme, globex, etc.). Any new tenants provisioned by the web server will also receive this update automatically.

-----

## 🔑 API Reference

The plugin exposes the following components for integration:

  * **`multitenancy.NewPlugin(config)`:** Registers the runtime callbacks and auto-migrates the `public.tenants` table.
  * **`multitenancy.TenantMiddleware()`:** Gin middleware for context injection.
  * **`multitenancy.NewTenantProvisioner(db, baseDSN, migrationsDir)`:** Tool used internally by your service to create a new tenant schema and migrate it.
  * **`multitenancy.LockDBConnection(db)`:** Security helper.
  * **`gmt-migrate` (Binary):** The dedicated command-line tool for mass schema upgrades/downgrades.
      * Flags: `-dsn-env` and `-migrations-env` allow overriding the default environment variable names.