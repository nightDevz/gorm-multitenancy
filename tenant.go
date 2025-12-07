package multitenancy

import (
	"context"
	"errors"
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"
)

// CtxKey is a custom type for context keys to prevent collisions.
type CtxKey string

// TenantSchemaKey is the context key used to store the tenant's schema name.
const TenantSchemaKey CtxKey = "tenant_schema"

var (
	// ErrInvalidTenantID is returned when the tenant ID contains illegal characters.
	ErrInvalidTenantID = errors.New("invalid tenant ID format")
	// ErrTenantNotFoundInContext is returned when the tenant schema is not found in context.
	ErrTenantNotFoundInContext = errors.New("tenant schema not found in context")
)

// safeSchemaRegex validates the schema name.
// We strictly allow only alphanumeric characters and underscores.
var safeSchemaRegex = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// SanitizeSchemaName ensures the schema name is safe for use in SQL.
func SanitizeSchemaName(name string) (string, error) {
	if !safeSchemaRegex.MatchString(name) {
		return "", ErrInvalidTenantID
	}
	return name, nil
}

// TenantMiddleware extracts the 'X-Tenant-ID' header, sanitizes it,
// and injects the schema name into the request context.
func TenantMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := c.GetHeader("X-Tenant-ID")
		if tenantID == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "X-Tenant-ID header is required"})
			return
		}

		safeSchemaName, err := SanitizeSchemaName(tenantID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Inject sanitized name into the request context.
		ctx := context.WithValue(c.Request.Context(), TenantSchemaKey, safeSchemaName)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}
