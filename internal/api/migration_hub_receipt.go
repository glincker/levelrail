package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/datamigrate"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

const secretMask = "********"

// buildHubConnection assembles the connection details of a managed database.
// ok is false only when a reveal is asked for and secrets are unavailable.
func (rt *Router) buildHubConnection(ctx context.Context, engine, name string, reveal bool) (hubConnection, bool) {
	host := application.DatabaseHost(name, rt.meshZone)
	port, _ := database.ContainerPort(engine)
	conn := hubConnection{
		Database: name, Engine: engine, Host: host, Port: port, Username: name, Name: name,
		Reference: name + ".url", Revealed: reveal,
	}
	password := secretMask
	if reveal {
		key, hasKey := database.PasswordSecretKey(engine)
		if rt.secrets == nil || !hasKey {
			return conn, false
		}
		pw, err := rt.secrets.Resolve(ctx, name, key)
		if err != nil {
			rt.logger.Warn("api: migration hub: resolve password failed", slog.String("error", err.Error()), slog.String("database", name))
			return conn, false
		}
		password = pw
	}
	if reveal {
		u := url.URL{Scheme: engine, User: url.UserPassword(name, password), Host: host + ":" + strconv.Itoa(port), Path: "/" + name}
		conn.URL = u.String()
	} else {
		conn.URL = fmt.Sprintf("%s://%s:%s@%s:%d/%s", engine, name, secretMask, host, port, name)
	}
	conn.Env = []hubEnvVar{
		{EnvVar: "DATABASE_URL", Field: "url", Value: conn.URL, Secret: true},
		{EnvVar: "DB_HOST", Field: "host", Value: host},
		{EnvVar: "DB_PORT", Field: "port", Value: strconv.Itoa(port)},
		{EnvVar: "DB_USER", Field: "username", Value: name},
		{EnvVar: "DB_NAME", Field: "database", Value: name},
		{EnvVar: "DB_PASSWORD", Field: "password", Value: password, Secret: true},
	}
	return conn, true
}

type receiptTable struct {
	Name   string `json:"name"`
	Source int64  `json:"source"`
	Target int64  `json:"target"`
}

type receiptDatabase struct {
	SourceDatabase string         `json:"source_database"`
	TargetDatabase string         `json:"target_database"`
	TargetVersion  string         `json:"target_version"`
	VerifiedAt     string         `json:"verified_at"`
	TablesChecked  int            `json:"tables_checked"`
	TablesDiffer   int            `json:"tables_differ"`
	Tables         []receiptTable `json:"tables"`
	Host           string         `json:"target_host"`
	Port           int            `json:"target_port"`
	Username       string         `json:"target_username"`
	Env            []hubEnvVar    `json:"env"`
}

type hubReceipt struct {
	GeneratedAt  string            `json:"generated_at"`
	SessionID    string            `json:"session_id"`
	SourceEngine string            `json:"source_engine"`
	SourceHost   string            `json:"source_host"`
	SourcePort   int               `json:"source_port"`
	SourceServer string            `json:"source_server_version"`
	SourceAccess string            `json:"source_access"`
	Statement    string            `json:"statement"`
	Databases    []receiptDatabase `json:"databases"`
}

// handleHubReceipt handles GET .../sessions/{id}/receipt. It holds no secret:
// env entries that are secret carry a mask, never a value.
func (rt *Router) handleHubReceipt(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, err := rt.migrationHub.GetMigrationSession(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: migration hub: load session failed", err, slog.String("session_id", id))
		return
	}
	items, err := rt.migrationHub.ListMigrationItems(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: migration hub: load items failed", err, slog.String("session_id", id))
		return
	}
	out := hubReceipt{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339), SessionID: id, SourceEngine: sess.Engine,
		SourceHost: sess.Host, SourcePort: sess.Port, SourceServer: sess.ServerVersion, SourceAccess: "read-only",
		Statement: "Only read-only statements and dump tools ran against the source. Nothing was written to or removed from it.",
		Databases: []receiptDatabase{},
	}
	for _, it := range items {
		if it.Status != store.HubItemVerified {
			continue
		}
		conn, _ := rt.buildHubConnection(r.Context(), sess.Engine, it.TargetName, false)
		d := receiptDatabase{
			SourceDatabase: it.SourceDB, TargetDatabase: it.TargetName, TargetVersion: it.TargetVersion,
			TablesChecked: it.Checked, TablesDiffer: it.Mismatched, Tables: []receiptTable{},
			Host: conn.Host, Port: conn.Port, Username: conn.Username, Env: maskedEnv(conn.Env),
		}
		if !it.FinishedAt.IsZero() {
			d.VerifiedAt = it.FinishedAt.UTC().Format(time.RFC3339)
		}
		var v datamigrate.Verification
		if json.Unmarshal([]byte(it.Detail), &v) == nil {
			for _, t := range v.Tables {
				d.Tables = append(d.Tables, receiptTable{Name: t.Name, Source: t.Source, Target: t.Target})
			}
		}
		out.Databases = append(out.Databases, d)
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="migration-receipt-%s.json"`, id))
	writeJSON(w, http.StatusOK, out)
}

func maskedEnv(in []hubEnvVar) []hubEnvVar {
	out := make([]hubEnvVar, 0, len(in))
	for _, e := range in {
		if e.Secret {
			e.Value = ""
		}
		out = append(out, e)
	}
	return out
}
