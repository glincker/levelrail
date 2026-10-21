package repolist

import (
	"context"
	"strings"
	"sync"

	"github.com/GLINCKER/levelrail/internal/store"
)

// MemoryStore is a Store that keeps snapshots in process memory, for a
// control plane whose main store cannot persist them and for tests.
type MemoryStore struct {
	mu sync.Mutex
	m  map[string]store.GitRepoListCacheEntry
}

// NewMemoryStore returns an empty MemoryStore.
func NewMemoryStore() *MemoryStore { return &MemoryStore{m: map[string]store.GitRepoListCacheEntry{}} }

// GetGitRepoListCache implements Store.
func (s *MemoryStore) GetGitRepoListCache(_ context.Context, key string) (store.GitRepoListCacheEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[key]
	if !ok {
		return store.GitRepoListCacheEntry{}, store.ErrGitRepoListCacheMiss
	}
	return e, nil
}

// PutGitRepoListCache implements Store.
func (s *MemoryStore) PutGitRepoListCache(_ context.Context, e store.GitRepoListCacheEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[e.Key] = e
	return nil
}

// DeleteGitRepoListCachePrefix implements Store.
func (s *MemoryStore) DeleteGitRepoListCachePrefix(_ context.Context, prefix string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.m {
		if strings.HasPrefix(k, prefix) {
			delete(s.m, k)
		}
	}
	return nil
}
