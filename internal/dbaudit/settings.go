package dbaudit

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// ServiceSettings adapts the live settings service.
type ServiceSettings struct{ Svc *runtimesettings.Service }

// StoreText reports whether statement text may be stored.
func (s ServiceSettings) StoreText(ctx context.Context) bool {
	return s.Svc.Bool(ctx, runtimesettings.KeyDatabasesAuditStoreText)
}

// QueueSize is the live queue bound.
func (s ServiceSettings) QueueSize(ctx context.Context) int {
	return s.Svc.Int(ctx, runtimesettings.KeyDatabasesAuditQueueSize)
}

// RetentionDays is the live retention window.
func (s ServiceSettings) RetentionDays(ctx context.Context) int {
	return s.Svc.Int(ctx, runtimesettings.KeyDatabasesAuditRetentionDays)
}
