package api

import (
	"encoding/json"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/store"
)

// promoteEnvChange is the env diff a protected promotion snapshots at
// request time, so approval applies what the requester saw.
type promoteEnvChange struct {
	Added   map[string]string `json:"added,omitempty"`
	Removed []string          `json:"removed,omitempty"`
}

func promoteEnvSnapshot(source, target store.DesiredService, include bool) (string, error) {
	if !include {
		return "", nil
	}
	d := buildPromoteDiff(source, target)
	c := promoteEnvChange{Added: make(map[string]string, len(d.EnvAdded)), Removed: d.EnvRemoved}
	for _, k := range d.EnvAdded {
		c.Added[k] = source.Env[k]
	}
	b, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("marshal promote env snapshot: %w", err)
	}
	return string(b), nil
}

// applyPromoteEnvSnapshot applies a request-time env diff. Keys the target
// gained since the request keep their current value.
func applyPromoteEnvSnapshot(target *store.DesiredService, snapshot string) error {
	if snapshot == "" {
		return nil
	}
	var c promoteEnvChange
	if err := json.Unmarshal([]byte(snapshot), &c); err != nil {
		return fmt.Errorf("decode promote env snapshot: %w", err)
	}
	env := make(map[string]string, len(target.Env)+len(c.Added))
	for k, v := range target.Env {
		env[k] = v
	}
	for _, k := range c.Removed {
		delete(env, k)
	}
	for k, v := range c.Added {
		if _, exists := env[k]; !exists {
			env[k] = v
		}
	}
	target.Env = env
	return nil
}
