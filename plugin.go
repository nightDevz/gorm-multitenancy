package multitenancy

import (
	"context"
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
	// 1. If no context is present, we cannot determine the tenant.
	if db.Statement.Context == nil {
		return
	}

	// 2. Try to get the tenant schema from the context.
	schema, ok := db.Statement.Context.Value(p.config.TenantKey).(string)
	if !ok || schema == "" {
		return
	}

	// 3. Optimization: Check if we've already set the path for this specific GORM statement/scope.
	if _, ok := db.Statement.Get("multitenancy:search_path_set"); ok {
		return
	}

	// 4. Sanitize the schema name to prevent SQL injection.
	safeSchemaName, err := p.config.Sanitizer(schema)
	if err != nil {
		_ = db.AddError(fmt.Errorf("gorm-multitenancy: %w", err))
		return
	}

	// 5. Connection Safety Check
	// We strictly require a Transaction. 'SET LOCAL' applies only to the current transaction.
	// If run on a standard pooled connection, the path change might leak or fail to apply to subsequent queries.
	if _, ok := db.Statement.ConnPool.(txCommitter); !ok {
		log.Printf("[gorm-multitenancy] ⚠️  WARNING: Tenant operation on schema '%s' is NOT running in a transaction! "+
			"This may cause data leaks. Please use multitenancy.ReadWriteTransaction(...) or multitenancy.ReadOnlyTransaction(...).", safeSchemaName)
	}

	// 6. Execute Schema Switch
	// We use a specific pattern here to prevent "Callback Recursion" (Stack Overflow).

	// A. Create a clean context. If GORM triggers this callback again recursively,
	//    it will see this empty context and exit immediately (Step 2).
	cleanCtx := context.Background()

	// B. Construct the query using SET LOCAL.
	query := fmt.Sprintf("SET LOCAL search_path TO %s, public", safeSchemaName)

	// C. Execute using a new Session with SkipHooks.
	//    - Context: cleanCtx (Circuit breaker for recursion)
	//    - SkipHooks: true (Performance optimization, tells GORM not to trigger plugins for this internal command)
	if err := db.Session(&gorm.Session{Context: cleanCtx, SkipHooks: true}).Exec(query).Error; err != nil {
		_ = db.AddError(fmt.Errorf("gorm-multitenancy: failed to set search_path: %w", err))
		return
	}

	// 7. Mark this statement as processed.
	db.Statement.Set("multitenancy:search_path_set", true)
}
