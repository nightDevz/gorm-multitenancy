package multitenancy

import (
	"fmt"

	"gorm.io/gorm"
)

// Sanitizer defines the function signature for sanitizing schema names.
type Sanitizer func(string) (string, error)

// Config holds the plugin's configuration.
type Config struct {
	// TenantKey is the context key used to retrieve the tenant schema.
	// e.g., kit.TenantSchemaKey
	TenantKey any
	// Sanitizer is the function used to validate the schema name.
	// e.g., kit.SanitizeSchemaName
	Sanitizer Sanitizer
}

// Plugin implements the gorm.Plugin interface.
type Plugin struct {
	config Config
}

// NewPlugin creates a new instance of the multitenancy plugin.
func NewPlugin(config Config) *Plugin {
	if config.TenantKey == nil {
		panic("gorm-multitenancy: TenantKey cannot be nil")
	}
	if config.Sanitizer == nil {
		panic("gorm-multitenancy: Sanitizer cannot be nil")
	}
	return &Plugin{config: config}
}

// Name returns the name of the plugin.
func (p *Plugin) Name() string {
	return "GormMultitenancyPlugin"
}

// Initialize registers the GORM callbacks.
// This version is corrected to avoid the undefined type error.
func (p *Plugin) Initialize(db *gorm.DB) error {
	// Register the callback for Create operations
	if err := db.Callback().Create().Before("gorm:create").
		Register("multitenancy:set_search_path", p.setSearchPathCallback); err != nil {
		return fmt.Errorf("failed to register create callback: %w", err)
	}

	// Register the callback for Query operations
	if err := db.Callback().Query().Before("gorm:query").
		Register("multitenancy:set_search_path", p.setSearchPathCallback); err != nil {
		return fmt.Errorf("failed to register query callback: %w", err)
	}

	// Register the callback for Update operations
	if err := db.Callback().Update().Before("gorm:update").
		Register("multitenancy:set_search_path", p.setSearchPathCallback); err != nil {
		return fmt.Errorf("failed to register update callback: %w", err)
	}

	// Register the callback for Delete operations
	if err := db.Callback().Delete().Before("gorm:delete").
		Register("multitenancy:set_search_path", p.setSearchPathCallback); err != nil {
		return fmt.Errorf("failed to register delete callback: %w", err)
	}

	// Register the callback for Row operations
	if err := db.Callback().Row().Before("gorm:row").
		Register("multitenancy:set_search_path", p.setSearchPathCallback); err != nil {
		return fmt.Errorf("failed to register row callback: %w", err)
	}

	// Register the callback for Raw operations
	if err := db.Callback().Raw().Before("gorm:raw").
		Register("multitenancy:set_search_path", p.setSearchPathCallback); err != nil {
		return fmt.Errorf("failed to register raw callback: %w", err)
	}

	return nil
}

// setSearchPathCallback is the core logic that sets the search_path.
func (p *Plugin) setSearchPathCallback(db *gorm.DB) {
	// 1. If no context is present, do nothing.
	if db.Statement.Context == nil {
		return
	}

	// 2. Try to get the tenant schema from the context.
	schema, ok := db.Statement.Context.Value(p.config.TenantKey).(string)
	if !ok || schema == "" {
		// No tenant key found. This is a "public" query
		// (e.g., login, provision). Do nothing.
		return
	}

	// 3. Check if we've already set this for the transaction.
	// This is a crucial optimization to avoid running SET search_path
	// multiple times in a single transaction.
	if _, ok := db.Statement.Get("multitenancy:search_path_set"); ok {
		return
	}

	// 4. Sanitize the schema name.
	safeSchemaName, err := p.config.Sanitizer(schema)
	if err != nil {
		_ = db.AddError(fmt.Errorf("gorm-multitenancy: %w", err))
		return
	}

	// 5. Execute the query to set the search_path for this connection/transaction.
	if err := db.Exec("SET search_path TO ?, public", safeSchemaName).Error; err != nil {
		_ = db.AddError(fmt.Errorf("gorm-multitenancy: failed to set search_path: %w", err))
		return
	}

	// 6. Mark this statement/transaction as "done"
	db.Statement.Set("multitenancy:search_path_set", true)
}
