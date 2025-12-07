package multitenancy

import (
	"log"

	"gorm.io/gorm"
)

// LockDBConnection sets the search_path to a non-existent schema
// to prevent accidental cross-tenant queries on the master connection.
// This is a security measure for the connection pool initialization.
func LockDBConnection(db *gorm.DB) {
	// We set the path to an unquoted identifier to ensure it fails
	// if someone tries to query without setting a tenant context.
	if err := db.Exec("SET search_path TO non_existent_schema_lock").Error; err != nil {
		log.Fatalf("❌ Failed to lock master connection search_path: %v", err)
	}
	log.Println("[gorm-multitenancy] ✅ Master database connection successful and locked!")
}
