package extdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
)

const (
	helperLabel      = "external-database-helper"
	helperTTLSeconds = "300"
	removeTimeout    = 20 * time.Second
	envHost          = "EXT_HOST"
	envPort          = "EXT_PORT"
	envUser          = "EXT_USER"
	envPassword      = "EXT_PASSWORD"
	envDatabase      = "EXT_DB"
	envAuthDB        = "EXT_AUTHDB"
	envTLS           = "EXT_TLS"
)

// HelperEnv is the environment a helper container runs with. Standard client
// variables are set too so commands never put the password in argv.
func HelperEnv(c Conn) map[string]string {
	env := map[string]string{
		envHost:     c.Host,
		envPort:     strconv.Itoa(c.Port),
		envUser:     c.User,
		envPassword: c.Password,
		envDatabase: c.Database,
		envAuthDB:   c.AuthDatabase,
		envTLS:      c.TLSMode,
	}
	switch c.Engine {
	case EnginePostgres:
		env["PGHOST"], env["PGPORT"], env["PGUSER"], env["PGPASSWORD"] = c.Host, strconv.Itoa(c.Port), c.User, c.Password
		env["PGDATABASE"] = c.Database
		env["PGSSLMODE"] = c.TLSMode
		env["PGCONNECT_TIMEOUT"] = "10"
	case EngineMySQL, EngineMariaDB:
		env["MYSQL_PWD"] = c.Password
	case EngineRedis:
		env["REDISCLI_AUTH"] = c.Password
	}
	return env
}

// Helper starts short-lived containers that run an engine's own client
// against a remote database. Nothing stays running: each helper is removed
// as soon as its caller is done, and expires on its own after a few minutes.
type Helper struct {
	Runtime docker.Runtime
	Logger  *slog.Logger
}

func (h *Helper) logger() *slog.Logger {
	if h.Logger != nil {
		return h.Logger
	}
	return slog.Default()
}

// Start creates and starts a helper for c and returns its container id and a
// remove function that is safe to call more than once.
func (h *Helper) Start(ctx context.Context, name string, c Conn) (string, func(), error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, fmt.Errorf("extdb: generate helper name: %w", err)
	}
	c = ResolveSource(ctx, h.Runtime, c)
	image := database.ImageRef(c.Engine, "")
	if c.SourceContainer != "" {
		if st, err := findContainer(ctx, h.Runtime, c.SourceContainer); err == nil && st != nil && st.Image != "" {
			image = st.Image
		}
	}
	spec := docker.ContainerSpec{
		Name:       "extdb-" + hex.EncodeToString(buf),
		Image:      image,
		Entrypoint: []string{"sleep"},
		Command:    []string{helperTTLSeconds},
		Env:        HelperEnv(c),
		Labels:     map[string]string{helperLabel: name},
	}
	if c.Network != "" && !builtinNetworks[c.Network] {
		spec.Network = &docker.NetworkAttachment{Name: c.Network}
	}
	id, err := h.Runtime.Create(ctx, spec)
	if err != nil {
		return "", nil, fmt.Errorf("extdb: create helper container: %s", c.Scrub(err.Error()))
	}
	var once sync.Once
	remove := func() {
		once.Do(func() { h.removeHelper(id, name, c) })
	}
	if err := h.Runtime.Start(ctx, id); err != nil {
		remove()
		return "", nil, fmt.Errorf("extdb: start helper container: %s", c.Scrub(err.Error()))
	}
	return id, remove, nil
}

func (h *Helper) removeHelper(id, name string, c Conn) {
	rmCtx, cancel := context.WithTimeout(context.Background(), removeTimeout)
	defer cancel()
	if rmErr := h.Runtime.Remove(rmCtx, id, true); rmErr != nil {
		h.logger().Warn("extdb: remove helper container failed", slog.String("database", name), slog.String("error", c.Scrub(rmErr.Error())))
	}
}

// Execer adapts a Helper to the viewer's Execer: every call starts a helper,
// runs the command in it, and removes it when the stream is closed.
type Execer struct {
	Helper *Helper
	Name   string
	Conn   Conn
}

// ExecWithInput implements dbviewer.Execer. The container id is ignored: the
// helper this call creates is the only target.
func (e *Execer) ExecWithInput(ctx context.Context, _ string, cmd []string, stdin io.Reader) (io.ReadCloser, error) {
	id, remove, err := e.Helper.Start(ctx, e.Name, e.Conn)
	if err != nil {
		return nil, err
	}
	rc, err := e.Helper.Runtime.ExecWithInput(ctx, id, cmd, stdin)
	if err != nil {
		remove()
		return nil, fmt.Errorf("extdb: exec in helper: %s", e.Conn.Scrub(err.Error()))
	}
	return &removeOnClose{ReadCloser: rc, remove: remove}, nil
}

type removeOnClose struct {
	io.ReadCloser
	remove func()
}

func (r *removeOnClose) Close() error {
	err := r.ReadCloser.Close()
	r.remove()
	return err
}
