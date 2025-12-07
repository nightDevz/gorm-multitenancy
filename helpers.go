package multitenancy

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

// ReadOnlyTransaction starts a transaction specifically routed to the Read Replica.
//
// Usage: Use this for List/Find/Get operations.
//
// Benefits:
// 1. Pins the connection to ensure 'SET LOCAL search_path' applies to your query.
// 2. Forces GORM to use the Read Replica (if available via dbresolver) to reduce load on the Writer.
// 3. Gracefully falls back to the Writer if dbresolver is not configured.
func ReadOnlyTransaction(ctx context.Context, db *gorm.DB, fc func(tx *gorm.DB) error) error {
	return db.Clauses(dbresolver.Read).WithContext(ctx).Transaction(fc)
}

// ReadWriteTransaction starts a standard transaction routed to the Writer (Source).
//
// Usage: Use this for Create/Update/Delete operations.
//
// Benefits:
// 1. Provides a consistent API alongside ReadOnlyTransaction.
// 2. Ensures the 'SET LOCAL search_path' command is applied safely before any writes occur.
func ReadWriteTransaction(ctx context.Context, db *gorm.DB, fc func(tx *gorm.DB) error) error {
	// Standard GORM Transactions always default to the Writer.
	return db.WithContext(ctx).Transaction(fc)
}
