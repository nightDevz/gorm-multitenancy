package multitenancy

import (
	"time"
)

// PublicTenant is the GORM model for the central tenant registry.
// It is stored in the 'public' schema.
type PublicTenant struct {
	ID         uint      `gorm:"primaryKey"`
	SchemaName string    `gorm:"column:schema_name;type:varchar(63);not null;unique"`
	IsActive   bool      `gorm:"column:is_active;type:boolean;not null;default:true"`
	CreatedAt  time.Time `gorm:"not null;default:now()"`
}

// TableName explicitly sets the table name to 'public.tenants'.
func (PublicTenant) TableName() string {
	return "public.tenants"
}
