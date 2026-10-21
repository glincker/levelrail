package api

import (
	"time"

	"github.com/GLINCKER/levelrail/internal/repolist"
)

func newRepoCacheForTest() *repolist.Cache {
	return repolist.NewCache(repolist.NewMemoryStore(), repolist.Config{
		TTL: 5 * time.Minute, StaleMax: 24 * time.Hour, RefreshMin: 0, MaxItems: 10000,
	}, discardLogger())
}
