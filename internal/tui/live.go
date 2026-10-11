package tui

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const deployListLimit = 100

// LiveAPI implements API over the HTTP client, one batched list call per section.
type LiveAPI struct {
	Client  *apiclient.Client
	Timeout time.Duration
}

func (l LiveAPI) ctx(parent context.Context) (context.Context, context.CancelFunc) {
	if l.Timeout <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, l.Timeout)
}

// sections runs one goroutine per section and collects failures by name.
type sections struct {
	wg   sync.WaitGroup
	mu   sync.Mutex
	errs map[string]string
}

func newSections() *sections { return &sections{errs: map[string]string{}} }

func (s *sections) run(name string, fn func() error) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := fn(); err != nil {
			s.mu.Lock()
			s.errs[name] = err.Error()
			s.mu.Unlock()
		}
	}()
}

func (s *sections) wait() map[string]string {
	s.wg.Wait()
	return s.errs
}

// Fast fetches apps, deployments and the attention feed concurrently.
func (l LiveAPI) Fast(parent context.Context) Fast {
	var out Fast
	ctx, cancel := l.ctx(parent)
	defer cancel()
	s := newSections()
	s.run(SecApps, func() error {
		v, err := l.Client.ListAppEntries(ctx)
		out.Apps = appRows(v)
		return err
	})
	s.run(SecDeploys, func() error {
		v, err := l.Client.ListDeployments(ctx, apiclient.DeploymentListOptions{Limit: deployListLimit})
		out.Deploys = deployRows(v.Items)
		return err
	})
	s.run(SecAttention, func() error {
		v, err := l.Client.GetAttentionFeed(ctx)
		out.Attention = attentionRows(v)
		return err
	})
	out.Errs = s.wait()
	return out
}

// Slow fetches databases, nodes, domains, backups, mesh and server version.
func (l LiveAPI) Slow(parent context.Context) Slow {
	var out Slow
	ctx, cancel := l.ctx(parent)
	defer cancel()
	s := newSections()
	s.run(SecDatabases, func() error {
		v, err := l.Client.ListDatabases(ctx)
		out.Databases = dbRows(v)
		return err
	})
	s.run(SecNodes, func() error {
		v, err := l.Client.ListNodes(ctx)
		out.Nodes = nodeRows(v)
		return err
	})
	s.run(SecDomains, func() error {
		v, err := l.Client.ListDomains(ctx)
		out.Domains = domainRows(v)
		return err
	})
	s.run(SecBackups, func() error {
		v, err := l.Client.ListAllBackups(ctx, apiclient.ListBackupsOptions{Limit: deployListLimit})
		out.Backups = backupRows(v)
		return err
	})
	s.run(SecMesh, func() error {
		v, err := l.Client.GetMeshStatus(ctx)
		if err != nil {
			// A 501 means mesh networking is off, which is not a fault.
			return nil
		}
		out.Mesh = meshInfo(v)
		return nil
	})
	s.run(SecServer, func() error {
		v, err := l.Client.GetUpdates(ctx)
		out.Server = ServerInfo{Version: v.CurrentVersion, UpdateAvailable: v.UpdateAvailable}
		return err
	})
	out.Errs = s.wait()
	return out
}

// Restart restarts app.
func (l LiveAPI) Restart(ctx context.Context, app string) error {
	_, err := l.Client.RestartApp(ctx, app)
	return wrapAction("restart", app, err)
}

// Start starts a stopped app.
func (l LiveAPI) Start(ctx context.Context, app string) error {
	_, err := l.Client.StartApp(ctx, app)
	return wrapAction("start", app, err)
}

// Stop stops app.
func (l LiveAPI) Stop(ctx context.Context, app string) error {
	_, err := l.Client.StopApp(ctx, app)
	return wrapAction("stop", app, err)
}

// Deploy redeploys app with image.
func (l LiveAPI) Deploy(ctx context.Context, app, image string) error {
	_, err := l.Client.DeployAppWith(ctx, app, apiclient.DeployTriggerRequest{Image: image, Confirm: true})
	return wrapAction("deploy", app, err)
}

// Rollback redeploys a past succeeded deploy of app.
func (l LiveAPI) Rollback(ctx context.Context, app, deployID string) error {
	_, err := l.Client.RollbackToDeploy(ctx, app, deployID, apiclient.RollbackToRequest{Confirm: true})
	return wrapAction("rollback", app, err)
}

// Logs tails an app or database log stream until ctx ends.
func (l LiveAPI) Logs(ctx context.Context, kind LogKind, name string, onLine func(LogLine) error) error {
	fn := func(e apiclient.LogStreamEntry) error {
		return onLine(LogLine{Text: e.Line, Stream: e.Stream})
	}
	if kind == LogDatabase {
		return l.Client.StreamDatabaseLogs(ctx, name, fn)
	}
	return l.Client.StreamLogs(ctx, name, fn)
}

func wrapAction(verb, app string, err error) error {
	if err != nil {
		return fmt.Errorf("%s %s: %w", verb, app, err)
	}
	return nil
}
