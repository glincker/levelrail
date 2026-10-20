package dataexport

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const splunkToken = "11111111-2222-3333-4444-555555555555"

type hecSink struct {
	mu       sync.Mutex
	bodies   []string
	auth     []string
	okLimit  int
	requests int
}

func (h *hecSink) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var rd io.Reader = r.Body
	if r.Header.Get("Content-Encoding") == "gzip" {
		zr, err := gzip.NewReader(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		rd = zr
	}
	b, _ := io.ReadAll(rd)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.requests++
	if h.okLimit > 0 && len(h.bodies) >= h.okLimit {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	h.bodies = append(h.bodies, string(b))
	h.auth = append(h.auth, r.Header.Get("Authorization"))
	_, _ = io.WriteString(w, `{"text":"Success","code":0}`)
}

func (h *hecSink) events() []splunkEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []splunkEvent
	for _, b := range h.bodies {
		for _, l := range strings.Split(strings.TrimSpace(b), "\n") {
			var e splunkEvent
			if json.Unmarshal([]byte(l), &e) == nil {
				out = append(out, e)
			}
		}
	}
	return out
}

func seedChainedAudit(t *testing.T, db *store.DB, n int) {
	t.Helper()
	base := time.Now().UTC().Add(-time.Hour)
	for i := 0; i < n; i++ {
		saveAudit(t, db, "c"+string(rune('a'+i)), base.Add(time.Duration(i)*time.Second), "/api/v1/apps/web/deploy")
	}
}

func TestSplunkSinkDeliversWithTokenMetadataAndChainFields(t *testing.T) {
	db := openDB(t)
	rcv := &hecSink{}
	srv := httptest.NewServer(rcv)
	defer srv.Close()
	seedChainedAudit(t, db, 2)
	svc := newTestService(t, db, nil)
	d, err := svc.Create(context.Background(), Input{
		Name: "splunk", Kind: store.ExportKindAuditSplunk, Endpoint: srv.URL, Enabled: true, APIKey: splunkToken,
		Options: store.ExportOptions{Backfill: true, Index: "audit", Source: "lr", SourceType: "lr:audit", Gzip: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Options.HeaderNames[0] != "Authorization" {
		t.Fatalf("token header not recorded: %+v", d.Options)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go svc.Run(ctx)
	waitFor(t, "two events", func() bool { return len(rcv.events()) == 2 })
	ev := rcv.events()[0]
	if ev.Index != "audit" || ev.Source != "lr" || ev.SourceType != "lr:audit" || ev.Event.ID != "ca" {
		t.Fatalf("bad event: %+v", ev)
	}
	if ev.Event.ChainSeq == 0 || ev.Event.ChainHash == "" {
		t.Fatalf("hash chain fields missing for a SIEM: %+v", ev.Event)
	}
	rcv.mu.Lock()
	defer rcv.mu.Unlock()
	if rcv.auth[0] != "Splunk "+splunkToken {
		t.Fatalf("auth header = %q", rcv.auth[0])
	}
	for _, b := range rcv.bodies {
		if strings.Contains(b, splunkToken) {
			t.Fatal("token leaked into the event body")
		}
	}
}

func TestSplunkHalfSuccessAdvancesCursorOnlyForAcceptedBatch(t *testing.T) {
	db := openDB(t)
	rcv := &hecSink{okLimit: 1}
	srv := httptest.NewServer(rcv)
	defer srv.Close()
	seedChainedAudit(t, db, 4)
	cfg := fastConfig()
	cfg.AuditBatch = 2
	t.Setenv(AllowPrivateEnv, "true")
	sec := &memSecrets{}
	svc := NewService(Deps{Store: db, Secrets: sec, Config: cfg, Brand: "plat"})
	d, err := svc.Create(context.Background(), Input{
		Name: "splunk", Kind: store.ExportKindAuditSplunk, Endpoint: srv.URL, Enabled: true, APIKey: splunkToken,
		Options: store.ExportOptions{Backfill: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go svc.Run(ctx)
	waitFor(t, "second batch failing", func() bool {
		st, _ := svc.Status(ctx, d.ID)
		return st.FailuresTotal >= 2 && st.CursorID == "cb"
	})
	cancel()
	svc.stopAll()
	st, _ := db.GetExportStatus(context.Background(), d.ID)
	if st.CursorID != "cb" || len(rcv.events()) != 2 {
		t.Fatalf("cursor = %q events = %d, want cb and 2", st.CursorID, len(rcv.events()))
	}

	rcv.mu.Lock()
	rcv.okLimit = 0
	rcv.mu.Unlock()
	svc2 := NewService(Deps{Store: db, Secrets: sec, Config: cfg, Brand: "plat"})
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	go svc2.Run(ctx2)
	waitFor(t, "resume delivers the rest exactly once", func() bool { return len(rcv.events()) == 4 })
	ids := []string{}
	for _, e := range rcv.events() {
		ids = append(ids, e.Event.ID)
	}
	if strings.Join(ids, ",") != "ca,cb,cc,cd" {
		t.Fatalf("ids = %v, want ca,cb,cc,cd with no duplicates after restart", ids)
	}
}

func TestSplunkTLSVerifyIsOnByDefault(t *testing.T) {
	rcv := &hecSink{}
	srv := httptest.NewTLSServer(rcv)
	defer srv.Close()
	db := openDB(t)
	svc := newTestService(t, db, nil)
	d, err := svc.Create(context.Background(), Input{
		Name: "splunk", Kind: store.ExportKindAuditSplunk, Endpoint: srv.URL, APIKey: splunkToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = svc.Test(context.Background(), d.ID)
	if err == nil || ClassifyTestError(err) != FixTLS {
		t.Fatalf("err = %v fix = %q, want a TLS failure by default", err, ClassifyTestError(err))
	}
	on := true
	opts := d.Options
	opts.InsecureSkipVerify = on
	if _, err := svc.Update(context.Background(), d.ID, Patch{Options: &opts}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Test(context.Background(), d.ID); err != nil {
		t.Fatalf("insecure opt-in should pass a self-signed collector: %v", err)
	}
}

func TestSplunkBadTokenClassified(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"text":"Invalid token","code":4}`)
	}))
	defer srv.Close()
	svc := newTestService(t, openDB(t), nil)
	d, err := svc.Create(context.Background(), Input{Name: "splunk", Kind: store.ExportKindAuditSplunk, Endpoint: srv.URL, APIKey: splunkToken})
	if err != nil {
		t.Fatal(err)
	}
	err = svc.Test(context.Background(), d.ID)
	if got := ClassifyTestError(err); got != FixBadToken {
		t.Fatalf("fix = %q (%v), want bad_token", got, err)
	}
}

func TestClassifyTestError(t *testing.T) {
	tests := []struct {
		err  error
		want FixCode
	}{
		{nil, FixNone},
		{errors.New("splunk hec returned status 401: x"), FixBadToken},
		{errors.New("x509: certificate signed by unknown authority"), FixTLS},
		{errors.New("splunk hec returned status 400: bad"), FixRejected},
		{errors.New("dial tcp 10.0.0.1:514: connect: connection refused"), FixUnreachable},
		{ErrBlockedDestination, FixPrivateBlocked},
		{errors.New("splunk hec returned status 500: x"), FixNone},
	}
	for _, tt := range tests {
		if got := ClassifyTestError(tt.err); got != tt.want {
			t.Errorf("ClassifyTestError(%v) = %q, want %q", tt.err, got, tt.want)
		}
	}
}

func readOctetFrames(t *testing.T, c net.Conn, count int) []string {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	br := bufio.NewReader(c)
	var out []string
	for len(out) < count {
		var n int
		for {
			b, err := br.ReadByte()
			if err != nil {
				t.Fatalf("read frame header: %v", err)
			}
			if b == ' ' {
				break
			}
			n = n*10 + int(b-'0')
		}
		buf := make([]byte, n)
		if _, err := io.ReadFull(br, buf); err != nil {
			t.Fatalf("read frame: %v", err)
		}
		out = append(out, string(buf))
	}
	return out
}

func sampleAuditBatch() auditBatch {
	rec := NewAuditRecord(store.AuditEntry{
		ID: "aud_1", ActorType: "token", ActorID: "tok_1", ActorName: `ci "bot"]`, Ability: "write", Method: "POST",
		Path: "/api/v1/apps/web/deploy", StatusCode: 403, RemoteAddr: "203.0.113.9:4000", CreatedAt: "2026-10-19T10:00:00.000000000Z",
		ClientKind: "cli", ChainSeq: 7, ChainPrev: "p", ChainHash: "h", ChainAlg: "k",
	})
	return auditBatch{records: []AuditRecord{rec}, firstID: "aud_1", lastID: "aud_1"}
}

func TestSyslogAuditOverTCPCarriesStructuredData(t *testing.T) {
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
		got <- readOctetFrames(t, c, 1)[0]
	}()
	s, err := newSyslogAuditSender("tcp://"+ln.Addr().String(), "local3", "lr-audit", "plat", false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	if err := s.sendAudit(context.Background(), sampleAuditBatch()); err != nil {
		t.Fatal(err)
	}
	select {
	case line := <-got:
		for _, want := range []string{
			"<156>1 2026-10-19T10:00:00.000000Z", " lr-audit - aud_1 [audit@32473 ", `id="aud_1"`, `actor_type="token"`,
			`actor="ci \"bot\"\]"`, `ability="write"`, `method="POST"`, `path="/api/v1/apps/web/deploy"`, `status="403"`,
			`remote_addr="203.0.113.9:4000"`, `surface="cli"`, `chain_seq="7"`, `chain_hash="h"`,
		} {
			if !strings.Contains(line, want) {
				t.Fatalf("message missing %q:\n%s", want, line)
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no syslog message")
	}
}

func TestSyslogAuditOverUDP(t *testing.T) {
	t.Setenv(AllowPrivateEnv, "true")
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pc.Close() }()
	s, err := newSyslogAuditSender("udp://"+pc.LocalAddr().String(), "", "", "plat", false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	if err := s.sendAudit(context.Background(), sampleAuditBatch()); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4096)
	_ = pc.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, _, err := pc.ReadFrom(buf)
	if err != nil {
		t.Fatal(err)
	}
	if msg := string(buf[:n]); !strings.HasPrefix(msg, "<84>1 ") || !strings.Contains(msg, " plat - aud_1 [audit@32473 ") {
		t.Fatalf("unexpected datagram: %s", msg)
	}
}

func TestSyslogAuditTLSVerifyAndInsecureOptIn(t *testing.T) {
	t.Setenv(AllowPrivateEnv, "true")
	cert := httptest.NewTLSServer(http.NotFoundHandler())
	defer cert.Close()
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: cert.TLS.Certificates, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	got := make(chan string, 1)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = c.Close() }()
				if err := c.(*tls.Conn).Handshake(); err != nil {
					return
				}
				got <- readOctetFrames(t, c, 1)[0]
			}()
		}
	}()
	secure, err := newSyslogAuditSender("tls://"+ln.Addr().String(), "", "", "plat", false)
	if err != nil {
		t.Fatal(err)
	}
	defer secure.close()
	err = secure.sendAudit(context.Background(), sampleAuditBatch())
	if err == nil || ClassifyTestError(err) != FixTLS {
		t.Fatalf("err = %v, want certificate verification failure", err)
	}
	loose, _ := newSyslogAuditSender("tls://"+ln.Addr().String(), "", "", "plat", true)
	defer loose.close()
	if err := loose.sendAudit(context.Background(), sampleAuditBatch()); err != nil {
		t.Fatalf("insecure opt-in: %v", err)
	}
	select {
	case <-got:
	case <-time.After(3 * time.Second):
		t.Fatal("no message over tls")
	}
}

func TestOTLPAuditSinkUsesBrandServiceNameAndSecretHeaders(t *testing.T) {
	var mu sync.Mutex
	var body []byte
	var hdr http.Header
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		body, hdr, path = b, r.Header.Clone(), r.URL.Path
		mu.Unlock()
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()
	svc := newTestService(t, openDB(t), nil)
	d, err := svc.Create(context.Background(), Input{
		Name: "otel", Kind: store.ExportKindAuditOTLP, Endpoint: srv.URL, Headers: map[string]string{"X-Api-Key": "s3cr3t-value"},
		Options: store.ExportOptions{Labels: map[string]string{"deployment_environment": "prod"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Test(context.Background(), d.ID); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if path != "/v1/logs" || hdr.Get("X-Api-Key") != "s3cr3t-value" {
		t.Fatalf("path = %q key = %q", path, hdr.Get("X-Api-Key"))
	}
	var doc struct {
		ResourceLogs []struct {
			Resource struct {
				Attributes []struct {
					Key   string `json:"key"`
					Value struct {
						StringValue string `json:"stringValue"`
					} `json:"value"`
				} `json:"attributes"`
			} `json:"resource"`
			ScopeLogs []struct {
				LogRecords []struct {
					Attributes []struct {
						Key   string `json:"key"`
						Value struct {
							IntValue string `json:"intValue"`
						} `json:"value"`
					} `json:"attributes"`
				} `json:"logRecords"`
			} `json:"scopeLogs"`
		} `json:"resourceLogs"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("payload is not OTLP JSON: %v\n%s", err, body)
	}
	attrs := map[string]string{}
	for _, a := range doc.ResourceLogs[0].Resource.Attributes {
		attrs[a.Key] = a.Value.StringValue
	}
	if attrs["service.name"] != "plat" || attrs["deployment_environment"] != "prod" {
		t.Fatalf("resource attributes = %v", attrs)
	}
	status := ""
	for _, a := range doc.ResourceLogs[0].ScopeLogs[0].LogRecords[0].Attributes {
		if a.Key == "http.response.status_code" {
			status = a.Value.IntValue
		}
	}
	if status != "200" || strings.Contains(string(body), "s3cr3t-value") {
		t.Fatalf("status = %q or header secret leaked", status)
	}
}

func TestAuditSinkValidationAndSSRF(t *testing.T) {
	svc := NewService(Deps{Store: openDB(t), Secrets: &memSecrets{}, Config: fastConfig(), Brand: "plat"})
	ctx := context.Background()
	tests := []struct {
		name string
		in   Input
	}{
		{"splunk without token", Input{Name: "a", Kind: store.ExportKindAuditSplunk, Endpoint: "https://203.0.113.10:8088"}},
		{"splunk private", Input{Name: "a", Kind: store.ExportKindAuditSplunk, Endpoint: "https://10.0.0.5:8088", APIKey: splunkToken}},
		{"splunk metadata", Input{Name: "a", Kind: store.ExportKindAuditSplunk, Endpoint: "http://169.254.169.254/x", APIKey: splunkToken}},
		{"syslog bad facility", Input{Name: "a", Kind: store.ExportKindAuditSyslog, Endpoint: "udp://203.0.113.10:514", Options: store.ExportOptions{Facility: "bogus"}}},
		{"syslog private", Input{Name: "a", Kind: store.ExportKindAuditSyslog, Endpoint: "tcp://192.168.1.2:514"}},
		{"syslog bad scheme", Input{Name: "a", Kind: store.ExportKindAuditSyslog, Endpoint: "http://203.0.113.10:514"}},
		{"syslog headers", Input{Name: "a", Kind: store.ExportKindAuditSyslog, Endpoint: "udp://203.0.113.10:514", Headers: map[string]string{"X-A": "b"}}},
		{"otlp private", Input{Name: "a", Kind: store.ExportKindAuditOTLP, Endpoint: "http://127.0.0.1:4318"}},
		{"facility on webhook", Input{Name: "a", Kind: store.ExportKindAuditWebhook, Endpoint: "https://203.0.113.10/x", Options: store.ExportOptions{Facility: "local0"}}},
	}
	for _, tt := range tests {
		if _, err := svc.Create(ctx, tt.in); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", tt.name, err)
		}
	}
	ok := Input{Name: "good", Kind: store.ExportKindAuditSyslog, Endpoint: "udp://203.0.113.10:514", Options: store.ExportOptions{Facility: "local0"}}
	if _, err := svc.Create(ctx, ok); err != nil {
		t.Fatalf("valid syslog audit sink rejected: %v", err)
	}

	t.Setenv(AllowPrivateEnv, "true")
	if _, err := svc.Create(ctx, Input{Name: "internal", Kind: store.ExportKindAuditSplunk, Endpoint: "https://10.0.0.5:8088", APIKey: splunkToken}); err != nil {
		t.Fatalf("explicit private opt-in should allow an internal Splunk: %v", err)
	}
	if _, err := svc.Create(ctx, Input{Name: "meta", Kind: store.ExportKindAuditSplunk, Endpoint: "http://169.254.169.254/x", APIKey: splunkToken}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("metadata address must stay refused with the opt-in set: %v", err)
	}
}

func TestDialGuardAlwaysRefusesMetadataAddress(t *testing.T) {
	t.Setenv(AllowPrivateEnv, "true")
	_, err := guardedDialer().DialContext(context.Background(), "tcp", "169.254.169.254:80")
	if !errors.Is(err, ErrBlockedDestination) {
		t.Fatalf("err = %v, want ErrBlockedDestination", err)
	}
}

func TestAuditRecordMasksSecretShapedPathsAndCarriesChain(t *testing.T) {
	r := NewAuditRecord(store.AuditEntry{
		ID: "x", Path: "/hooks/ghp_" + "abcdefghijklmnopqrstuvwxyz0123456789", ChainSeq: 3, ChainHash: "hh", ChainPrev: "pp", ChainAlg: "k",
	})
	if strings.Contains(r.Path, "ghp_") || r.ChainSeq != 3 || r.ChainHash != "hh" {
		t.Fatalf("record = %+v", r)
	}
}
