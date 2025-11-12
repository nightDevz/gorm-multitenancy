package multitenancy

import (
	"context"
	"errors"
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"
)

// CtxKey is a custom type for context keys.
type CtxKey string

// TenantSchemaKey is the key used to store the tenant's schema name.
const TenantSchemaKey CtxKey = "tenant_schema"

var (
	// ErrInvalidTenantID is returned when the tenant ID has an invalid format.
	ErrInvalidTenantID = errors.New("invalid tenant ID format")
	// ErrTenantNotFoundInContext is returned when the tenant schema is not found.
	ErrTenantNotFoundInContext = errors.New("tenant schema not found in context")
)

// safeSchemaRegex validates the schema name.
// We only allow alphanumeric characters and underscores.
var safeSchemaRegex = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// SanitizeSchemaName ensures the schema name is safe for use in SQL.
func SanitizeSchemaName(name string) (string, error) {
	if !safeSchemaRegex.MatchString(name) {
		return "", ErrInvalidTenantID
	}
	return name, nil
}

// TenantMiddleware extracts, sanitizes, and injects the tenant schema.
func TenantMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := c.GetHeader("X-Tenant-ID")
		if tenantID == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "X-Tenant-ID header is required"})
			return
		}

		safeSchemaName, err := SanitizeSchemaName(tenantID)
		if err != nil {
			// We can be sure err is ErrInvalidTenantID
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Set the sanitized name in the request's context.
		ctx := context.WithValue(c.Request.Context(), TenantSchemaKey, safeSchemaName)
		c.Request = c.Request.WithContext(ctx)

		// The c.Set() call is removed. The context is the single source of truth.

		c.Next()
	}
}
