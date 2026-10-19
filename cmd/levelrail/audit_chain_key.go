package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	auditChainKeyService = "audit/chain-key"
	auditChainKeyEnvKey  = "key"
)

// installAuditChainKey loads, or creates on first boot, the random key audit
// rows are chained under. It is wrapped by the master key like any secret, so a
// master key rotation keeps it and a database copy alone cannot forge the chain.
func installAuditChainKey(ctx context.Context, logger *slog.Logger, sm *secrets.Manager) {
	if sm == nil {
		return
	}
	key, err := sm.Resolve(ctx, auditChainKeyService, auditChainKeyEnvKey)
	if errors.Is(err, secrets.ErrValueNotFound) {
		key, err = newAuditChainKey()
		if err == nil {
			err = sm.SetValue(ctx, auditChainKeyService, auditChainKeyEnvKey, key)
		}
	}
	if err != nil {
		logger.Warn("audit chain key unavailable, new audit rows are chained with a plain hash", slog.String("error", err.Error()))
		return
	}
	store.SetAuditChainKey([]byte(key))
}

func newAuditChainKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate audit chain key: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
