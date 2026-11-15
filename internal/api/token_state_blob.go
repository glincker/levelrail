package api

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

const (
	tokenStateService  = "token-lifecycle"
	tokenStateNotices  = "expiry_notices" //nolint:gosec // state key, not a credential
	tokenStateRotation = "rotation_pending"
)

// tokenStateBook keeps small id to number maps (reminder levels, pending
// revocations) in the secrets store so they survive a restart. Without a
// master key it holds them in memory only.
type tokenStateBook struct {
	mu  sync.Mutex
	mem map[string]map[string]int64
	// rotating holds the token ids with a rotation in flight, so one token has at most one.
	rotating map[string]bool
	// graceFailing is set while the rotation grace sweep cannot run, for the attention feed.
	graceFailing bool
}

func (b *tokenStateBook) setGraceFailing(v bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.graceFailing = v
}

func (b *tokenStateBook) isGraceFailing() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.graceFailing
}

// claimRotation marks id as rotating; the returned release must run when the rotation ends.
func (b *tokenStateBook) claimRotation(id string) (func(), bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.rotating[id] {
		return nil, false
	}
	if b.rotating == nil {
		b.rotating = map[string]bool{}
	}
	b.rotating[id] = true
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		delete(b.rotating, id)
	}, true
}

func (rt *Router) tokenStateLoad(ctx context.Context, key string) (map[string]int64, error) {
	rt.tokenState.mu.Lock()
	defer rt.tokenState.mu.Unlock()
	return rt.tokenStateLoadLocked(ctx, key)
}

func (rt *Router) tokenStateLoadLocked(ctx context.Context, key string) (map[string]int64, error) {
	out := map[string]int64{}
	if rt.gitConnSecrets == nil {
		for k, v := range rt.tokenState.mem[key] {
			out[k] = v
		}
		return out, nil
	}
	ok, err := rt.gitConnSecrets.Exists(ctx, tokenStateService, key)
	if err != nil || !ok {
		if err != nil {
			return out, fmt.Errorf("api: token state exists: %w", err)
		}
		return out, nil
	}
	raw, err := rt.gitConnSecrets.Resolve(ctx, tokenStateService, key)
	if err != nil {
		return out, fmt.Errorf("api: token state resolve: %w", err)
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]int64{}, fmt.Errorf("api: token state decode: %w", err)
	}
	return out, nil
}

// tokenStateUpdate applies fn to the stored map and saves the result.
func (rt *Router) tokenStateUpdate(ctx context.Context, key string, fn func(map[string]int64)) error {
	rt.tokenState.mu.Lock()
	defer rt.tokenState.mu.Unlock()
	cur, err := rt.tokenStateLoadLocked(ctx, key)
	if err != nil {
		return err
	}
	fn(cur)
	if rt.gitConnSecrets == nil {
		if rt.tokenState.mem == nil {
			rt.tokenState.mem = map[string]map[string]int64{}
		}
		rt.tokenState.mem[key] = cur
		return nil
	}
	b, err := json.Marshal(cur)
	if err != nil {
		return fmt.Errorf("api: token state encode: %w", err)
	}
	if err := rt.gitConnSecrets.SetValue(ctx, tokenStateService, key, string(b)); err != nil {
		return fmt.Errorf("api: token state save: %w", err)
	}
	return nil
}
