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

	// 2. Use fmt.Sprintf for the CREATE SCHEMA DDL command.
	createSchemaQuery := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", safeSchemaName)
	if err := r.db.WithContext(ctx).Exec(createSchemaQuery).Error; err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	// Create a new session for this specific tenant's migration
	tenantDB := r.db.WithContext(ctx).Session(&gorm.Session{NewDB: true})

	// 3. Use fmt.Sprintf for the SET search_path command.
	// This is the line that fixes the new error.
	setSearchPathQuery := fmt.Sprintf("SET search_path TO %s, public", safeSchemaName)
	if err := tenantDB.Exec(setSearchPathQuery).Error; err != nil {
		return fmt.Errorf("failed to set search_path for migration: %w", err)
	}

	// 4. Run AutoMigrate for all provided models
	if err := tenantDB.AutoMigrate(models...); err != nil {
		return fmt.Errorf("failed to auto-migrate tenant schema: %w", err)
	}

	return nil
}
