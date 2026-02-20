package multitenancy

import (
	"fmt"
	"log"

	"gorm.io/gorm"
)

// Sanitizer defines the function signature for sanitizing schema names.
type Sanitizer func(string) (string, error)

// Config holds the plugin's configuration.
type Config struct {
	// TenantKey is the context key used to retrieve the tenant schema.
	// e.g., multitenancy.TenantSchemaKey
	TenantKey any
	// Sanitizer is the function used to validate the schema name.
	// e.g., multitenancy.SanitizeSchemaName
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

// txCommitter is a local interface used to verify if the current DB connection
// is a transaction. Standard *sql.DB connections do not implement this.
type txCommitter interface {
	Commit() error
	Rollback() error
}

// Initialize registers the GORM callbacks and AutoMigrates the registry table.
func (p *Plugin) Initialize(db *gorm.DB) error {
	// 1. Auto-migrate the public.tenants table immediately.
	log.Println("[gorm-multitenancy] Checking for 'public.tenants' table...")
	if err := db.AutoMigrate(&PublicTenant{}); err != nil {
		return fmt.Errorf("gorm-multitenancy: failed to auto-migrate public.tenants: %w", err)
	}
	log.Println("[gorm-multitenancy] 'public.tenants' table is ready.")

	// 2. Register callbacks.
	// We use "Before" hooks to ensure the search_path is set immediately before any SQL runs.
	// We unroll the registration loop to ensure compatibility across different GORM versions.

	if err := db.Callback().Create().Before("gorm:create").Register("multitenancy:set_search_path", p.setSearchPathCallback); err != nil {
		return fmt.Errorf("failed to register create callback: %w", err)
	}
	if err := db.Callback().Query().Before("gorm:query").Register("multitenancy:set_search_path", p.setSearchPathCallback); err != nil {
		return fmt.Errorf("failed to register query callback: %w", err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register("multitenancy:set_search_path", p.setSearchPathCallback); err != nil {
		return fmt.Errorf("failed to register update callback: %w", err)
	}
	if err := db.Callback().Delete().Before("gorm:delete").Register("multitenancy:set_search_path", p.setSearchPathCallback); err != nil {
		return fmt.Errorf("failed to register delete callback: %w", err)
	}
	if err := db.Callback().Row().Before("gorm:row").Register("multitenancy:set_search_path", p.setSearchPathCallback); err != nil {
		return fmt.Errorf("failed to register row callback: %w", err)
	}
	if err := db.Callback().Raw().Before("gorm:raw").Register("multitenancy:set_search_path", p.setSearchPathCallback); err != nil {
		return fmt.Errorf("failed to register raw callback: %w", err)
	}

	return nil
}

// setSearchPathCallback attempts to extract the tenant from the context and sets the search_path.
func (p *Plugin) setSearchPathCallback(db *gorm.DB) {
	// 1. Context Safety: If no context, we can't do anything.
	if db.Statement.Context == nil {
		return
	}

	// 2. CIRCUIT BREAKER: Check if this specific statement is already processed
	// OR if this is an internal multitenancy command.
	if _, skip := db.Statement.Get("multitenancy:skip_callback"); skip {
		return
	}

	// 3. Extract Tenant
	schema, ok := db.Statement.Context.Value(p.config.TenantKey).(string)
	if !ok || schema == "" {
		return
	}

	// 4. Sanitize
	safeSchemaName, err := p.config.Sanitizer(schema)
	if err != nil {
		_ = db.AddError(fmt.Errorf("gorm-multitenancy: %w", err))
		return
	}

	// 5. THE FIX: Execute "Quietly"
	query := fmt.Sprintf("SET LOCAL search_path TO %s, public", safeSchemaName)

	// We create a new session and set "multitenancy:skip_callback" to true.
	// This ensures that when db.Exec runs, the plugin sees the flag and exits at Step 2.
	err = db.Session(&gorm.Session{NewDB: true}).
		Set("multitenancy:skip_callback", true).
		Exec(query).Error

	if err != nil {
		_ = db.AddError(fmt.Errorf("gorm-multitenancy: failed to set search_path: %w", err))
		return
	}

	// 6. Mark the original statement as done so we don't repeat this in the same chain
	db.Statement.Set("multitenancy:skip_callback", true)
}
