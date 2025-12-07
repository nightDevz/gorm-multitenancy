# gorm-multitenancy

Latest stable **v2.0.13**

A complete, production-grade solution for building multi-tenant applications with GORM using the **Schema-per-Tenant** strategy. This package provides automatic, secure, and request-scoped tenant isolation by manipulating PostgreSQL's `search_path`.

It is designed for high-scale environments (like **AWS Aurora**) and supports Read/Write splitting.



-----

## ⚠️ Critical Usage Requirement

**You MUST wrap all tenant-scoped operations in a Transaction.**

This plugin uses `SET LOCAL search_path` to ensure thread safety and prevent connection pool pollution. The schema change applies **only** to the current transaction.

If you run a query without a transaction, Go's connection pool might execute the `SET LOCAL` command on "Connection A" and your `SELECT` query on "Connection B", causing you to query the wrong schema (usually `public`).

### ⚡ Recommended Pattern (Transaction Helpers)

We provide built-in helpers to ensure your code is consistent, type-safe, and correctly routed to Writers or Readers (if using `dbresolver`).

#### 1. Reads (Routed to Replica)
Use `multitenancy.ReadOnlyTransaction` for fetching data. This allows GORM to route traffic to Read Replicas while maintaining tenant isolation.

```go
func (r *ProductRepository) List(ctx context.Context) ([]Product, error) {
    var products []Product
    
    // Automatically routes to Read Replica & sets schema safely
    err := multitenancy.ReadOnlyTransaction(ctx, r.db, func(tx *gorm.DB) error {
        return tx.Find(&products).Error
    })
    
    return products, err
}
````

#### 2\. Writes (Routed to Writer)

Use `multitenancy.ReadWriteTransaction` for changing data.

```go
func (r *ProductRepository) Create(ctx context.Context, p *Product) error {
    // Automatically routes to Writer & sets schema safely
    return multitenancy.ReadWriteTransaction(ctx, r.db, func(tx *gorm.DB) error {
        return tx.Create(p).Error
    })
}
```

-----

## ✨ Features

  * **Consistent Transaction API:** Helpers for `ReadOnly` and `ReadWrite` operations.
  * **Recursion Circuit Breaker:** Smart context scrubbing prevents GORM plugin infinite loops (Stack Overflow protection).
  * **Connection Pool Safety:** Uses `SET LOCAL` to ensure tenant paths never leak to other connections in the pool.
  * **Accidental Deletion Protection:** The provisioner performs strict orphan checks before creating schemas.
  * **Production-Safe CLI:** The migration tool requires confirmation before running `down` migrations.
  * **Auto-Initialized Registry:** Creates `public.tenants` automatically.
  * **Gin Middleware:** Extracts `X-Tenant-ID` and injects it into the context.

-----

## 🏛️ Architecture Overview

1.  **Request:** HTTP request carries `X-Tenant-ID`. Middleware injects it into `context`.
2.  **Isolation:** The plugin intercepts DB calls using that context. It creates a "Quiet Session" to execute `SET LOCAL search_path` on the pinned transaction connection.
3.  **Execution:** The user's query runs on the isolated schema.
4.  **Cleanup:** When the transaction ends (Commit/Rollback), Postgres automatically reverts the `search_path`, ensuring the connection is clean when returned to the pool.

-----

## 📦 Installation

```bash
# 1. Install the plugin library
go get [github.com/nightDevz/gorm-multitenancy/v2@v2.0.13](https://github.com/nightDevz/gorm-multitenancy/v2@v2.0.13)

# 2. Install dependencies
go get gorm.io/gorm gorm.io/plugin/dbresolver [github.com/gin-gonic/gin](https://github.com/gin-gonic/gin) [github.com/pressly/goose/v3](https://github.com/pressly/goose/v3)
go get gorm.io/driver/postgres [github.com/lib/pq](https://github.com/lib/pq)

# 3. Install the CLI migration tool
go install [github.com/nightDevz/gorm-multitenancy/v2/cmd/gmt-migrate@v2.0.13](https://github.com/nightDevz/gorm-multitenancy/v2/cmd/gmt-migrate@v2.0.13)
```

-----

## 🚀 Usage Guide

### Step 1: Initialize in `main.go`

```go
package main

import (
    "log"
    multitenancy "[github.com/nightDevz/gorm-multitenancy/v2](https://github.com/nightDevz/gorm-multitenancy/v2)"
    "gorm.io/gorm"
)

func main() {
    db := config.InitDB() // Setup GORM + dbresolver here if needed

    // Registering the plugin AUTOMATICALLY creates the public.tenants table.
    plugin := multitenancy.NewPlugin(multitenancy.Config{
        TenantKey: multitenancy.TenantSchemaKey,
        Sanitizer: multitenancy.SanitizeSchemaName,
    })
    
    if err := db.Use(plugin); err != nil {
        log.Fatalf("❌ Failed to register plugin: %v", err)
    }
}
```

### Step 2: Provisioning New Tenants

Use the `TenantProvisioner` service to create new schemas safely.

```go
provisioner := multitenancy.NewTenantProvisioner(db, "host=writer-endpoint...", "./migrations")

// This runs CREATE SCHEMA, registers the tenant, and runs initial migrations.
// It includes rollback logic if the migration fails.
err := provisioner.ProvisionTenant(ctx, "tenant_a")
```

### Step 3: Run Migrations (CLI)

Use the CLI tool to migrate all tenant schemas at once. You must provide the **Writer DSN**, as schemas cannot be created/altered on Read Replicas.

```bash
# Standard usage
export DB_WRITER_DSN="host=aurora-writer..." 
gmt-migrate -dsn-env=DB_WRITER_DSN up

# Rollback (Prompts for confirmation)
gmt-migrate -dsn-env=DB_WRITER_DSN down
```

-----

## 🛠️ Debugging

If you see the error:
`[gorm-multitenancy] ⚠️ WARNING: Tenant operation on schema 'x' is NOT running in a transaction!`

This means you called a GORM method (like `db.Find` or `db.Create`) directly on the DB object without wrapping it in a transaction. The plugin blocked this unsafe operation to prevent data leaks. **Switch to using `multitenancy.ReadOnlyTransaction` or `ReadWriteTransaction`.**

-----

## 🔑 API Reference

  * **`multitenancy.ReadOnlyTransaction(...)`**: Helper for Read Replica operations.
  * **`multitenancy.ReadWriteTransaction(...)`**: Helper for Writer operations.
  * **`multitenancy.NewPlugin(config)`**: Registers callbacks and auto-migrates the registry.
  * **`multitenancy.TenantMiddleware()`**: Gin middleware for context injection.
  * **`multitenancy.LockDBConnection(db)`**: Sets the master connection path to a non-existent schema for security.
  * **`gmt-migrate`**: Binary for mass schema updates.
