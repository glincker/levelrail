package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/brand"
	ingressdriver "github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/trafficexport"
)

// trafficExportSource loads the stored sinks and resolves their secrets for the export service.
type trafficExportSource struct {
	db  *store.DB
	sec *secrets.Manager
}

// LoadSpecs implements trafficexport.Source. A sink whose secrets cannot be read is skipped, not fatal.
func (s trafficExportSource) LoadSpecs(ctx context.Context) ([]trafficexport.SinkSpec, error) {
	rows, err := s.db.ListTrafficExportSinks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list sinks: %w", err)
	}
	out := make([]trafficexport.SinkSpec, 0, len(rows))
	for _, r := range rows {
		sec := map[string]string{}
		for _, slot := range trafficexport.SecretSlots(r.Kind) {
			v, err := s.sec.Resolve(ctx, store.TrafficExportSecretsKey(r.ID), slot)
			if err != nil {
				if !errors.Is(err, secrets.ErrValueNotFound) {
					slog.Warn("traffic export: secret unreadable", "sink", r.ID, "slot", slot)
				}
				continue
			}
			sec[slot] = v
		}
		out = append(out, trafficexport.SinkSpec{
			ID: r.ID, Name: r.Name, Kind: r.Kind, Enabled: r.Enabled, Host: r.Host, Fields: r.Fields,
			IPTruncate: r.IPTruncate, Sample: r.Sample, Secrets: sec,
		})
	}
	return out, nil
}

// visitorSalt returns the server secret behind visitor ids, creating it on first use.
func visitorSalt(sec *secrets.Manager) func(context.Context) ([]byte, error) {
	return func(ctx context.Context) ([]byte, error) {
		v, err := sec.Resolve(ctx, store.TrafficExportSaltService, store.TrafficExportSaltKey)
		if err == nil {
			return []byte(v), nil
		}
		if !errors.Is(err, secrets.ErrValueNotFound) {
			return nil, fmt.Errorf("read visitor salt: %w", err)
		}
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return nil, fmt.Errorf("generate visitor salt: %w", err)
		}
		v = fmt.Sprintf("%x", raw)
		if err := sec.SetValue(ctx, store.TrafficExportSaltService, store.TrafficExportSaltKey, v); err != nil {
			return nil, fmt.Errorf("store visitor salt: %w", err)
		}
		return []byte(v), nil
	}
}

// startTrafficExport taps the ingress access log and starts the configured sinks. It returns nil without a
// master key, because sink credentials can only be stored through the secrets manager.
func startTrafficExport(ctx context.Context, logger *slog.Logger, b *brand.Brand, db *store.DB, sec *secrets.Manager) *trafficexport.Service {
	if sec == nil {
		return nil
	}
	httpOpts := trafficexport.HTTPOptionsFromEnv()
	httpOpts.Namespace = b.ShortName
	stats := ingressdriver.DefaultRequestStats()
	svc := trafficexport.NewService(trafficExportSource{db: db, sec: sec}, httpOpts, trafficexport.OptionsFromEnv(), stats.OwnerOf, visitorSalt(sec), logger)
	svc.Install(ingressdriver.DefaultAccessLog())
	if err := svc.Reload(ctx); err != nil {
		logger.Warn("traffic export: initial load failed", slog.String("error", err.Error()))
	}
	return svc
}
