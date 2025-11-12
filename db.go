package multitenancy

import (
	"log"

	"gorm.io/gorm"
)

// LockDBConnection sets the search_path to a non-existent schema
// to prevent accidental cross-tenant queries on the master connection.
func LockDBConnection(db *gorm.DB) {
	// Corrected SQL: Identifiers like schema names should not be single-quoted.
	// This sets the path to an unquoted identifier.
	if err := db.Exec("SET search_path TO non_existent_schema_lock").Error; err != nil {
		log.Fatalf("❌ Failed to lock master connection search_path: %v", err)
	}
	log.Println("✅ Master database connection successful and locked!")
}
