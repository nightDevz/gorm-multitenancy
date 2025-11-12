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
	// 1. Sanitize the name. This is the most important security step.
	safeSchemaName, err := SanitizeSchemaName(schemaName)
	if err != nil {
		return err
	}

	// 2. Use fmt.Sprintf to build the DDL query.
	// This is safe ONLY because we are using the 'safeSchemaName'
	// variable which has been validated.
	// We CANNOT use a parameter '?' for a schema name.
	createSchemaQuery := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", safeSchemaName)
	if err := r.db.WithContext(ctx).Exec(createSchemaQuery).Error; err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	// Create a new session for this specific tenant's migration
	tenantDB := r.db.WithContext(ctx).Session(&gorm.Session{NewDB: true})

	// 3. Set the search_path for this new session.
	// This 'SET' command *can* use a parameter.
	if err := tenantDB.Exec("SET search_path TO ?, public", safeSchemaName).Error; err != nil {
		return fmt.Errorf("failed to set search_path for migration: %w", err)
	}

	// 4. Run AutoMigrate for all provided models
	if err := tenantDB.AutoMigrate(models...); err != nil {
		return fmt.Errorf("failed to auto-migrate tenant schema: %w", err)
	}

	return nil
}
