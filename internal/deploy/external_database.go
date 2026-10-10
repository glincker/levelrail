package deploy

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/store"
)

// externalDatabaseGetter is satisfied by *store.DB; fakes that lack it only
// see managed databases.
type externalDatabaseGetter interface {
	GetExternalDatabase(ctx context.Context, name string) (*store.ExternalDatabase, error)
}
