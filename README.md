gorm-multitenancy

Latest stable v2.0.11

A complete solution for building multi-tenant applications with GORM using the Schema-per-Tenant strategy. This package provides automatic, secure, and request-scoped tenant isolation by manipulating PostgreSQL's search_path.

⚠️ Critical Usage Requirement

You MUST wrap all tenant-scoped operations in a Transaction.

Because this plugin uses SET LOCAL search_path to guarantee thread-safety and prevent connection pool pollution, the schema change applies only to the current transaction.

If you run a query without a transaction, Go's connection pool might execute the SET LOCAL command on Connection A and your SELECT query on Connection B, causing you to query the wrong schema (usually public).

Correct Usage:

// Even for READ operations, use a transaction to pin the connection!
err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
    // 1. Plugin automatically runs "SET LOCAL search_path..." on 'tx'
    // 2. You run your query on 'tx'
    if err := tx.Find(&products).Error; err != nil {
        return err
    }
    return nil // Commit (Connection returned to pool, search_path resets)
})


✨ Features

Recursion Circuit Breaker: Uses smart context scrubbing to prevent GORM plugin infinite loops/stack overflows.

Connection Pool Safety: Uses SET LOCAL to ensure tenant paths never leak to other connections in the pool.

Auto-Initialized Registry: Automatically creates the central public.tenants registry table when initialized.

Built-in CLI Migration Tool: gmt-migrate binary with flexible flag support for microservices.

Gin Middleware: Extracts X-Tenant-ID and injects it into the context.

🏛️ Architecture Overview

This package relies on PostgreSQL schemas. Each tenant (e.g., "acme", "globex") gets their own isolated schema.

Request: HTTP request carries X-Tenant-ID. Middleware injects it into context.

GORM Interception: The plugin intercepts any DB call using that context.

Isolation: It executes SET LOCAL search_path TO <tenant_schema>, public on the current transaction.

Cleanup: When the transaction ends (Commit/Rollback), Postgres automatically reverts the search_path, ensuring the connection is clean when returned to the pool.

📦 Installation

# 1. Install the plugin library
go get [github.com/nightDevz/gorm-multitenancy/v2@v2.0.11](https://github.com/nightDevz/gorm-multitenancy/v2@v2.0.11)

# 2. Install dependencies
go get gorm.io/gorm [github.com/gin-gonic/gin](https://github.com/gin-gonic/gin) [github.com/pressly/goose/v3](https://github.com/pressly/goose/v3)
go get gorm.io/driver/postgres [github.com/lib/pq](https://github.com/lib/pq)

# 3. Install the CLI migration tool
go install [github.com/nightDevz/gorm-multitenancy/v2/cmd/gmt-migrate@v2.0.11](https://github.com/nightDevz/gorm-multitenancy/v2/cmd/gmt-migrate@v2.0.11)


🚀 Usage Guide

Step 1: Initialize in main.go

package main

import (
    "log"
    multitenancy "[github.com/nightDevz/gorm-multitenancy/v2](https://github.com/nightDevz/gorm-multitenancy/v2)"
    "gorm.io/gorm"
)

func main() {
    db := config.InitDB() 

    // Registering the plugin AUTOMATICALLY creates the public.tenants table.
    plugin := multitenancy.NewPlugin(multitenancy.Config{
        TenantKey: multitenancy.TenantSchemaKey,
        Sanitizer: multitenancy.SanitizeSchemaName,
    })
    
    if err := db.Use(plugin); err != nil {
        log.Fatalf("❌ Failed to register plugin: %v", err)
    }
}


Step 2: Implement Repositories (The Right Way)

Do not rely on implicit connections. Always use .Transaction().

internal/repository/product_repository.go

func (r *ProductRepository) Create(ctx context.Context, p *Product) error {
    // Passing 'ctx' triggers the plugin
    return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        if err := tx.Create(p).Error; err != nil {
            return err
        }
        return nil
    })
}

func (r *ProductRepository) List(ctx context.Context) ([]Product, error) {
    var products []Product
    
    // Even for reads! This ensures the "SET LOCAL" and "SELECT" happen on the same DB connection.
    err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        if err := tx.Find(&products).Error; err != nil {
            return err
        }
        return nil
    })
    
    return products, err
}


Step 3: Run Migrations

Use the CLI tool to migrate all tenant schemas at once.

# Standard usage
export DB_DSN="host=localhost..." 
gmt-migrate up

# Microservice usage (Override variable names)
gmt-migrate -dsn-env=BILLING_SERVICE_DSN up


🛠️ Debugging

If you see the error:
[gorm-multitenancy] ⚠️ WARNING: Tenant operation on schema 'x' is NOT running in a transaction!

This means you called a GORM method (like db.Find or db.Create) directly on the DB object without wrapping it in db.Transaction(...). The plugin blocked this potentially unsafe operation to prevent data leaks. Wrap your code in a transaction to fix it.

🔑 API Reference

multitenancy.NewPlugin(config): Registers callbacks and auto-migrates the registry.

multitenancy.TenantMiddleware(): Gin middleware for context injection.

multitenancy.NewTenantProvisioner(...): Helper to create new schemas.

gmt-migrate: Binary for mass schema updates.