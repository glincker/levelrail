package dataexport

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

type memSecrets struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *memSecrets) SetValue(_ context.Context, svc, key, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string]string{}
	}
	s.m[svc+"/"+key] = v
	return nil
}

func (s *memSecrets) Resolve(_ context.Context, svc, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[svc+"/"+key]
	if !ok {
		return "", errors.New("missing")
	}
	return v, nil
}

func (s *memSecrets) DeleteAll(_ context.Context, svc string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.m {
		if strings.HasPrefix(k, svc+"/") {
			delete(s.m, k)
		}
	}
	return nil
}

func openDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func fastConfig() Config {
	c := DefaultConfig()
	c.Resync, c.FlushInterval, c.AuditInterval, c.StatusInterval = 20*time.Millisecond, 10*time.Millisecond, 20*time.Millisecond, 20*time.Millisecond
	c.BackoffBase, c.BackoffMax, c.MaxAttempts, c.SendTimeout, c.BreakerCooldown = time.Millisecond, 5*time.Millisecond, 3, 2*time.Second, 50*time.Millisecond
	return c
}

func TestNewLogRecordLevels(t *testing.T) {
	tests := []struct {
		name  string
		entry telemetry.LogEntry
		level string
		app   string
		kind  string
	}{
		{"stdout plain", telemetry.LogEntry{ResourceID: "service:web", Stream: "stdout", Message: "hi"}, "info", "web", "app"},
		{"stderr plain", telemetry.LogEntry{ResourceID: "service:web", Stream: "stderr", Message: "x"}, "error", "web", "app"},
		{"structured warn", telemetry.LogEntry{ResourceID: "service:api", Stream: "stdout", Structured: true, FieldsJSON: `{"level":"WARNING"}`}, "warn", "api", "app"},
		{"structured severity", telemetry.LogEntry{ResourceID: "database:pg", Stream: "stdout", Structured: true, FieldsJSON: `{"severity":"fatal"}`}, "error", "pg", "database"},
		{"unknown level keeps stream default", telemetry.LogEntry{ResourceID: "service:a", Stream: "stdout", Structured: true, FieldsJSON: `{"level":"loud"}`}, "info", "a", "app"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewLogRecord(tt.entry)
			if r.Level != tt.level || r.App != tt.app || r.Kind != tt.kind {
				t.Fatalf("got level=%s app=%s kind=%s", r.Level, r.App, r.Kind)
			}
		})
	}
}

func TestLogRecordFilters(t *testing.T) {
	web := LogRecord{App: "web", Kind: "app", Level: "info"}
	tests := []struct {
		name     string
		rec      LogRecord
		apps     map[string]struct{}
		minLevel string
		want     bool
	}{
		{"no filters", web, nil, "", true},
		{"app match", web, map[string]struct{}{"web": {}}, "", true},
		{"app miss", web, map[string]struct{}{"api": {}}, "", false},
		{"level below", web, nil, "warn", false},
		{"level at", LogRecord{App: "web", Kind: "app", Level: "warn"}, nil, "warn", true},
		{"database excluded by app filter", LogRecord{App: "web", Kind: "database", Level: "info"}, map[string]struct{}{"web": {}}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rec.matches(tt.apps, tt.minLevel); got != tt.want {
				t.Fatalf("matches = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLokiPayloadGroupsStreamsAndBoundsLabels(t *testing.T) {
	s := &lokiSender{job: "plat", static: map[string]string{"env": "prod"}}
	ts := time.Unix(10, 5)
	body, err := s.payload([]LogRecord{
		{Time: ts, App: "web", Kind: "app", Stream: "stdout", Level: "info", Message: "a"},
		{Time: ts, App: "web", Kind: "app", Stream: "stdout", Level: "info", Message: "b"},
		{Time: ts, App: "web", Kind: "app", Stream: "stderr", Level: "error", Message: "c"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Streams []lokiStream `json:"streams"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Streams) != 2 || len(doc.Streams[0].Values) != 2 {
		t.Fatalf("streams = %+v, want 2 streams with 2 values in the first", doc.Streams)
	}
	for _, k := range []string{"job", "app", "kind", "stream", "level", "env"} {
		if _, ok := doc.Streams[0].Stream[k]; !ok {
			t.Fatalf("label %q missing: %v", k, doc.Streams[0].Stream)
		}
	}
	if len(doc.Streams[0].Stream) != 6 {
		t.Fatalf("unexpected labels: %v", doc.Streams[0].Stream)
	}
	if doc.Streams[0].Values[0][0] != "10000000005" {
		t.Fatalf("timestamp = %s", doc.Streams[0].Values[0][0])
	}
}

func TestLokiPushURL(t *testing.T) {
	tests := map[string]string{
		"http://loki:3100":                      "http://loki:3100/loki/api/v1/push",
		"http://loki:3100/":                     "http://loki:3100/loki/api/v1/push",
		"https://x.example/custom/push":         "https://x.example/custom/push",
		"https://logs.example/loki/api/v1/push": "https://logs.example/loki/api/v1/push",
	}
	for in, want := range tests {
		if got := LokiPushURL(in); got != want {
			t.Errorf("LokiPushURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHTTPLogSenderStatusClassification(t *testing.T) {
	tests := []struct {
		code      int
		wantErr   bool
		permanent bool
	}{{204, false, false}, {500, true, false}, {429, true, false}, {400, true, true}, {401, true, true}}
	for _, tt := range tests {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer k" {
				t.Errorf("secret header missing")
			}
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(tt.code)
		}))
		s := &httpLogSender{url: srv.URL, client: srv.Client(), headers: map[string]string{"Authorization": "Bearer k"}}
		err := s.send(context.Background(), []LogRecord{{Time: time.Now(), App: "a", Message: "m"}})
		var perm *permanentError
		if (err != nil) != tt.wantErr || errors.As(err, &perm) != tt.permanent {
			t.Errorf("code %d: err=%v permanent=%v", tt.code, err, errors.As(err, &perm))
		}
		srv.Close()
	}
}

func TestSyslogRFC5424OverTCP(t *testing.T) {
	t.Setenv(AllowPrivateEnv, "true")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	got := make(chan string, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
		br := bufio.NewReader(c)
		var n int
		var sb strings.Builder
		for {
			b, err := br.ReadByte()
			if err != nil || b == ' ' {
				break
			}
			sb.WriteByte(b)
		}
		for _, ch := range sb.String() {
			n = n*10 + int(ch-'0')
		}
		buf := make([]byte, n)
		_, _ = io.ReadFull(br, buf)
		got <- sb.String() + " " + string(buf)
	}()
	s, err := newSyslogSender("tcp://"+ln.Addr().String(), "plat")
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	rec := LogRecord{Time: time.Date(2026, 10, 11, 1, 2, 3, 0, time.UTC), App: "web", Stream: "stderr", Level: "error", Message: "boom"}
	if err := s.send(context.Background(), []LogRecord{rec}); err != nil {
		t.Fatalf("send: %v", err)
	}
	select {
	case line := <-got:
		for _, want := range []string{"<131>1 2026-10-11T01:02:03.000000Z", "plat.web", "stderr", " - boom"} {
			if !strings.Contains(line, want) {
				t.Fatalf("line %q missing %q", line, want)
			}
		}
		n, msg, _ := strings.Cut(line, " ")
		if n == "" || len(msg) == 0 {
			t.Fatalf("bad framing: %q", line)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no syslog message received")
	}
}

func TestSyslogEndpointValidation(t *testing.T) {
	for raw, ok := range map[string]bool{
		"tcp://logs.example.com:514": true, "tls://logs.example.com:6514": true,
		"udp://x:514": false, "tcp://x": false, "http://x:1": false,
	} {
		_, _, err := ParseSyslogEndpoint(raw)
		if (err == nil) != ok {
			t.Errorf("ParseSyslogEndpoint(%q) err=%v, want ok=%v", raw, err, ok)
		}
	}
}

func TestSSRFValidation(t *testing.T) {
	tests := []struct {
		endpoint string
		wantErr  bool
	}{
		{"http://127.0.0.1:3100", true},
		{"http://localhost:3100", true},
		{"http://10.0.0.5:3100", true},
		{"http://169.254.169.254/latest", true},
		{"http://[::1]:3100", true},
		{"ftp://example.com", true},
		{"https://user:pw@example.com", true},
		{"not a url", true},
		{"https://203.0.113.10/push", false},
	}
	for _, tt := range tests {
		t.Run(tt.endpoint, func(t *testing.T) {
			err := ValidateHTTPEndpoint(context.Background(), tt.endpoint)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
	t.Setenv(AllowPrivateEnv, "true")
	if err := ValidateHTTPEndpoint(context.Background(), "http://127.0.0.1:3100"); err != nil {
		t.Fatalf("allow switch should permit loopback: %v", err)
	}
}

func TestDialGuardBlocksInternalAddress(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	_, err = guardedDialer().DialContext(context.Background(), "tcp", ln.Addr().String())
	if !errors.Is(err, ErrBlockedDestination) {
		t.Fatalf("err = %v, want ErrBlockedDestination", err)
	}
}

func TestSignatureRoundTrip(t *testing.T) {
	key := "0123456789abcdef-key"
	body := []byte(`{"id":"a"}` + "\n")
	now := time.Unix(1_800_000_000, 0)
	h := Sign(key, now, body)
	tests := []struct {
		name   string
		key    string
		header string
		body   []byte
		at     time.Time
		wantOK bool
	}{
		{"valid", key, h, body, now, true},
		{"wrong key", "another-key-123456789", h, body, now, false},
		{"tampered body", key, h, append(body, 'x'), now, false},
		{"stale", key, h, body, now.Add(time.Hour), false},
		{"garbage", key, "nonsense", body, now, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifySignature(tt.key, tt.header, tt.body, tt.at, 5*time.Minute)
			if (err == nil) != tt.wantOK {
				t.Fatalf("err = %v, wantOK %v", err, tt.wantOK)
			}
		})
	}
}

func TestAuditRecordRedactsSecrets(t *testing.T) {
	r := NewAuditRecord(store.AuditEntry{
		ID: "a1", Path: "/api/v1/hooks/ghp_" + strings.Repeat("a", 36),
		ActorName: "Bearer abcdefghijklmnop", RemoteAddr: "10.0.0.1",
	})
	if strings.Contains(r.Path, "ghp_") || strings.Contains(r.ActorName, "abcdefghijklmnop") {
		t.Fatalf("secrets leaked: %+v", r)
	}
}

func TestPrometheusAndOTLPEncoding(t *testing.T) {
	at := time.Unix(100, 0)
	series := []Series{
		{Name: "p_cpu_percent", Help: "cpu", Labels: []Label{{"app", `we"b`}}, Value: 1.5, Time: at},
		{Name: "p_network_rx_bytes", Help: "rx", Counter: true, Labels: []Label{{"app", "web"}}, Value: 7, Time: at},
	}
	var sb strings.Builder
	if err := WritePrometheus(&sb, series); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# TYPE p_cpu_percent gauge", `p_cpu_percent{app="we\"b"} 1.5`, "# TYPE p_network_rx_bytes counter", `p_network_rx_bytes{app="web"} 7`} {
		if !strings.Contains(sb.String(), want) {
			t.Fatalf("exposition missing %q:\n%s", want, sb.String())
		}
	}
	sample := regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*(\{([a-zA-Z_][a-zA-Z0-9_]*="([^"\\]|\\.)*",?)*\})? [-+0-9.eE]+$`)
	for _, line := range strings.Split(strings.TrimSpace(sb.String()), "\n") {
		if strings.HasPrefix(line, "# HELP ") || strings.HasPrefix(line, "# TYPE ") {
			continue
		}
		if !sample.MatchString(line) {
			t.Fatalf("line is not valid exposition format: %q", line)
		}
	}
	raw, err := EncodeOTLP(series, "plat", at)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if !strings.Contains(string(raw), `"isMonotonic":true`) || !strings.Contains(string(raw), `"service.name"`) {
		t.Fatalf("unexpected otlp body: %s", raw)
	}
}

type fakeSource map[string][]telemetry.Sample

func (f fakeSource) LatestByMetric(_ context.Context, m string) ([]telemetry.Sample, error) {
	return f[m], nil
}

func TestCollectResourceSeriesBoundsLabelsAndFilters(t *testing.T) {
	src := fakeSource{"cpu_percent": {
		{ResourceID: "service:web", Value: 1}, {ResourceID: "database:pg", Value: 2},
		{ResourceID: "node:n1", Value: 3}, {ResourceID: "model:llama", Value: 4}, {ResourceID: "service:secret", Value: 5},
	}}
	got, err := CollectResourceSeries(context.Background(), src, "p_", func(_, name string) bool { return name != "secret" })
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("series = %+v, want app, database and node only", got)
	}
	for _, s := range got {
		if len(s.Labels) != 1 || (s.Labels[0].Name != "app" && s.Labels[0].Name != "database" && s.Labels[0].Name != "node") {
			t.Fatalf("unbounded label set: %+v", s.Labels)
		}
	}
}
