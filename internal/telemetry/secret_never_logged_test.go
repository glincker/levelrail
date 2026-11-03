package telemetry_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploylog"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/mcptools"
	"github.com/GLINCKER/levelrail/internal/secretmask"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// Synthetic values. The env value carries characters that change under URL encoding.
const (
	secretEnvValue  = "s3cr3t-value-for-test&k=v/x"
	secretBuildArg  = "s3cr3t-build-arg-for-test"
	neverLoggedMark = "never-logged-resource"
)

// echoLines are the shapes a tool echoes a secret in: raw, base64 header, URL-encoded DSN.
func echoLines(secret string) []string {
	return []string{
		"RUN export TOKEN=" + secret,
		"Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(secret)),
		"dsn=postgres://u:" + url.QueryEscape(secret) + "@db/x",
	}
}

// forms lists every spelling of a secret the masker claims to cover.
func forms(secret string) []string {
	return []string{
		secret,
		base64.StdEncoding.EncodeToString([]byte(secret)),
		url.QueryEscape(secret),
	}
}

func assertClean(t *testing.T, sink, observed string) {
	t.Helper()
	for _, secret := range []string{secretEnvValue, secretBuildArg} {
		for _, f := range forms(secret) {
			if strings.Contains(observed, f) {
				t.Errorf("%s leaked %q: %s", sink, f, observed)
			}
		}
	}
}

type captureStore struct {
	mu   sync.Mutex
	rows []telemetry.DeployLogEntry
}

func (c *captureStore) WriteDeployLogBatch(_ context.Context, e []telemetry.DeployLogEntry) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rows = append(c.rows, e...)
	return nil
}

func (c *captureStore) text() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var b strings.Builder
	for _, r := range c.rows {
		b.WriteString(r.Message + "\n")
	}
	return b.String()
}

type gatedSource struct {
	gate  <-chan struct{}
	lines []string
}

func (g gatedSource) Logs(context.Context, string, bool, time.Time) (<-chan docker.LogLine, <-chan error) {
	out := make(chan docker.LogLine)
	go func() {
		defer close(out)
		<-g.gate
		for _, l := range g.lines {
			out <- docker.LogLine{Stream: "stdout", Timestamp: time.Now(), Message: l}
		}
	}()
	return out, make(chan error)
}

func buildLogSink(t *testing.T, lines []string) string {
	st := &captureStore{}
	r := deploylog.NewRecorder(st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r.Start("dep_secret")
	_, live, unsub, ok := r.Snapshot("dep_secret")
	if !ok {
		t.Fatal("attempt not active")
	}
	defer unsub()
	var seen strings.Builder
	for _, l := range lines {
		r.Progress("dep_secret")(build.ProgressEvent{Log: l, Stream: "stdout"})
		seen.WriteString((<-live).Line + "\n")
	}
	r.Finish(t.Context(), "dep_secret")
	return seen.String() + st.text()
}

func auditSink(t *testing.T, lines []string) string {
	ctx := t.Context()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for i, l := range lines {
		e := store.AuditEntry{
			ID: "a" + string(rune('0'+i)), ActorType: "token", ActorID: "t1", ActorName: l, Ability: "env.write",
			Method: "PUT", Path: "/api/v1/apps/web/env?v=" + l, StatusCode: 200,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Detail: `{"value":"` + l + `"}`,
		}
		if err := db.SaveAuditEntry(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := db.ListAuditEntries(ctx, 50, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(entries)
	return string(out)
}

// collectLogs runs the real collector over lines and returns what the log store holds and what a drain received.
func collectLogs(t *testing.T, lines []string) (stored, drained string) {
	ctx := t.Context()
	db, err := telemetry.Open(ctx, filepath.Join(t.TempDir(), "telemetry.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var mu sync.Mutex
	var got strings.Builder
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got.Write(b)
		mu.Unlock()
	}))
	t.Cleanup(srv.Close)

	bc := telemetry.NewLogBroadcaster()
	discard := slog.New(slog.NewTextHandler(io.Discard, nil))
	fwd := telemetry.NewDrainForwarder(bc, srv.Client(), discard, "levelrail")
	fctx, stop := context.WithCancel(ctx)
	runDone := make(chan struct{})
	go func() {
		defer close(runDone)
		_ = fwd.Run(fctx, time.Hour, func(context.Context) ([]telemetry.DrainConfig, error) {
			return []telemetry.DrainConfig{{ResourceID: neverLoggedMark, Type: telemetry.DrainSinkHTTP, Target: srv.URL, Enabled: true}}, nil
		})
	}()

	// The forwarder must be subscribed before the first line is published.
	gate := make(chan struct{})
	time.AfterFunc(300*time.Millisecond, func() { close(gate) })
	lc := telemetry.NewLogCollector(gatedSource{gate: gate, lines: lines}, db, bc, discard)
	if err := lc.StreamOne(ctx, telemetry.LogTarget{ResourceID: neverLoggedMark, ContainerID: "c1", Stopped: true}); err != nil {
		t.Fatal(err)
	}
	stop()
	<-runDone
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := got.Len()
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	rows, err := db.QueryLogs(ctx, neverLoggedMark, time.Now().Add(-time.Hour), time.Now().Add(time.Hour), "")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(rows)
	mu.Lock()
	defer mu.Unlock()
	return string(raw), got.String()
}

func mcpAuditSink(t *testing.T, lines []string) string {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/audit-log" {
			http.NotFound(w, r)
			return
		}
		// The API serves rows from the audit store, so the tool sees what storage kept.
		var rows []apiclient.AuditLogEntryResource
		_ = json.Unmarshal([]byte(auditSink(t, lines)), &rows)
		_ = json.NewEncoder(w).Encode(rows)
	}))
	t.Cleanup(api.Close)
	server := mcptools.NewServer(apiclient.NewClient(api.URL, "test-token"))
	st, ct := mcp.NewInMemoryTransports()
	go func() { _ = server.Run(t.Context(), st) }()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil).Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "list_audit_log", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(res)
	return string(out)
}

func mcpEnvSink(t *testing.T) string {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/secrets"):
			_, _ = w.Write([]byte(`[{"key":"API_TOKEN"}]`))
		case strings.HasPrefix(r.URL.Path, "/api/v1/apps/web"):
			// The API masks secret values before they leave it.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "web", "image": "i", "secret_env": []string{"API_TOKEN"},
				"env": map[string]string{"MODE": "prod", "API_TOKEN": apiclient.MaskedEnvValue},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(api.Close)
	server := mcptools.NewServer(apiclient.NewClient(api.URL, "test-token"))
	st, ct := mcp.NewInMemoryTransports()
	go func() { _ = server.Run(t.Context(), st) }()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil).Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_app_env", Arguments: map[string]any{"name": "web"}})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(res)
	return string(out)
}

func TestSecretNeverLogged(t *testing.T) {
	// Registering is what the secrets manager does when an env secret or build secret is saved.
	secretmask.Add(secretEnvValue)
	secretmask.Add(secretBuildArg)
	var lines []string
	lines = append(lines, echoLines(secretEnvValue)...)
	lines = append(lines, echoLines(secretBuildArg)...)

	sinks := []struct {
		name string
		run  func(t *testing.T) string
	}{
		{"stored and live build log", func(t *testing.T) string { return buildLogSink(t, lines) }},
		{"audit entries", func(t *testing.T) string { return auditSink(t, lines) }},
		{"log store", func(t *testing.T) string { s, _ := collectLogs(t, lines); return s }},
		{"log drain", func(t *testing.T) string { _, d := collectLogs(t, lines); return d }},
		{"mcp list_audit_log", func(t *testing.T) string { return mcpAuditSink(t, lines) }},
		{"mcp get_app_env", mcpEnvSink},
	}
	for _, s := range sinks {
		t.Run(s.name, func(t *testing.T) {
			out := s.run(t)
			if out == "" {
				t.Fatal("sink produced no output to inspect")
			}
			assertClean(t, s.name, out)
		})
	}
}
