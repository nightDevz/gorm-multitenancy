package multitenancy

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// TenantProvisioner handles provisioning new tenants.
type TenantProvisioner struct {
	db *gorm.DB
}

// NewTenantProvisioner creates a new TenantProvisioner.
func NewTenantProvisioner(db *gorm.DB) *TenantProvisioner {
	return &TenantProvisioner{db: db}
}

// ProvisionTenant creates a schema and migrates it.
// It accepts the models to migrate as variadic arguments.
func (r *TenantProvisioner) ProvisionTenant(ctx context.Context, schemaName string, models ...interface{}) error {
	safeSchemaName, err := SanitizeSchemaName(schemaName)
	if err != nil {
		return err
	}

	// Use a parameterized query for DDL. GORM will handle quoting.
	// This creates: CREATE SCHEMA IF NOT EXISTS "safe_schema_name"
	if err := r.db.WithContext(ctx).Exec("CREATE SCHEMA IF NOT EXISTS ?", safeSchemaName).Error; err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	// Create a new session for this specific tenant's migration
	tenantDB := r.db.WithContext(ctx).Session(&gorm.Session{NewDB: true})

	// Set the search_path for this new session using parameters
	if err := tenantDB.Exec("SET search_path TO ?, public", safeSchemaName).Error; err != nil {
		return fmt.Errorf("failed to set search_path for migration: %w", err)
	}

	// Run AutoMigrate for all provided models within the tenant's schema
	if err := tenantDB.AutoMigrate(models...); err != nil {
		return fmt.Errorf("failed to auto-migrate tenant schema: %w", err)
	}

	return nil
}
