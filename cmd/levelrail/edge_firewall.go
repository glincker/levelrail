package main

import (
	"context"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/edgefw"
	"github.com/GLINCKER/levelrail/internal/secrets"
)

const edgeFirewallKeyPurpose = "edge-firewall-challenge"

// setupEdgeFirewall seals the challenge cookies with a key derived from the
// master key and loads the stored rules into the running ingress engine.
func setupEdgeFirewall(ctx context.Context, logger *slog.Logger, b *brand.Brand, router *api.Router, sm *secrets.Manager) {
	eng := edgefw.Default()
	eng.SetTitle(b.Name)
	if key := sm.DeriveKey(edgeFirewallKeyPurpose); key != nil {
		eng.Signer.SetKey(key)
	} else {
		logger.Warn("firewall challenge passes reset on restart: no master key loaded to derive their key")
	}
	if err := router.LoadEdgeFirewall(ctx); err != nil {
		logger.Error("firewall rules not loaded, the ingress firewall is empty", slog.String("error", err.Error()))
	}
}
