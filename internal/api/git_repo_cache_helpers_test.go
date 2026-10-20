package api

import (
	"time"

	"github.com/GLINCKER/levelrail/internal/repolist"
)

func newRepoCacheForTest(refreshMin time.Duration) *repolist.Cache {
	return repolist.NewCache(repolist.NewMemoryStore(), repolist.Config{
		TTL: 5 * time.Minute, StaleMax: 24 * time.Hour, RefreshMin: refreshMin, MaxItems: 10000,
	}, discardLogger())
}
