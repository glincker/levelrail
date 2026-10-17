package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/plugin"
	"github.com/GLINCKER/levelrail/internal/store"
)

var _ plugin.Containers = (*docker.Client)(nil)

// setupPlugins starts the plugin supervisor. It returns nil unless the plugins
// feature is on and Docker is reachable, so a default install runs none of it.
func setupPlugins(ctx context.Context, logger *slog.Logger, b *brand.Brand, db *store.DB, client *docker.Client, dataDir string) *plugin.Manager {
	if !experimental.Enabled(experimental.Plugins) || client == nil {
		return nil
	}
	cfg := plugin.ConfigFromEnv(b.ShortName, dataDir, os.LookupEnv)
	cfg.UID, cfg.GID = os.Getuid(), os.Getgid()
	mgr := plugin.NewManager(plugin.NewSQLStore(db), client, api.PluginAuditor{DB: db}, cfg, logger)
	go mgr.Run(ctx)
	return mgr
}

type pluginAlertSender struct{ m *plugin.Manager }

func (s pluginAlertSender) SendPlugin(ctx context.Context, name, secret string, n alerting.PluginNotice) error {
	d, err := s.m.Send(ctx, name, secret, plugin.Notification{
		ID: n.ID, Title: n.Title, Body: n.Body, Status: n.Status, Severity: n.Severity, ResourceID: n.ResourceID, At: n.At, SensitiveLines: n.LogLines,
	})
	if err != nil {
		return err
	}
	if !d.Delivered {
		return fmt.Errorf("plugin %q did not deliver: %s", name, d.Detail)
	}
	return nil
}

func pluginSenderFor(m *plugin.Manager) alerting.PluginSender {
	if m == nil {
		return nil
	}
	return pluginAlertSender{m: m}
}
