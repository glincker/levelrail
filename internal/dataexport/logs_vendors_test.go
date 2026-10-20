package dataexport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func TestVendorEndpoint(t *testing.T) {
	tests := []struct {
		kind    store.ExportKind
		opts    store.ExportOptions
		want    string
		wantErr bool
	}{
		{store.ExportKindLogsNewRelic, store.ExportOptions{}, "https://log-api.newrelic.com/log/v1", false},
		{store.ExportKindLogsNewRelic, store.ExportOptions{Region: "eu"}, "https://log-api.eu.newrelic.com/log/v1", false},
		{store.ExportKindLogsNewRelic, store.ExportOptions{Region: "mars"}, "", true},
		{store.ExportKindLogsDatadog, store.ExportOptions{}, "https://http-intake.logs.datadoghq.com/api/v2/logs", false},
		{store.ExportKindLogsDatadog, store.ExportOptions{Site: "us5.datadoghq.com"}, "https://http-intake.logs.us5.datadoghq.com/api/v2/logs", false},
		{store.ExportKindLogsDatadog, store.ExportOptions{Site: "evil.test/x"}, "", true},
		{store.ExportKindLogsAxiom, store.ExportOptions{Dataset: "prod logs"}, "https://api.axiom.co/v1/datasets/prod%20logs/ingest", false},
		{store.ExportKindLogsAxiom, store.ExportOptions{}, "", true},
		{store.ExportKindLogsBetterStack, store.ExportOptions{}, "https://in.logs.betterstack.com", false},
	}
	for _, tt := range tests {
		got, err := VendorEndpoint(tt.kind, tt.opts)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("VendorEndpoint(%s,%+v) = %q, %v; want %q (err=%v)", tt.kind, tt.opts, got, err, tt.want, tt.wantErr)
		}
	}
}

func TestVendorKeyHeaders(t *testing.T) {
	if h := vendorKeyHeaders(store.ExportKindLogsNewRelic, "k"); h["Api-Key"] != "k" {
		t.Errorf("newrelic header = %v", h)
	}
	if h := vendorKeyHeaders(store.ExportKindLogsAxiom, "k"); h["Authorization"] != "Bearer k" {
		t.Errorf("axiom header = %v", h)
	}
	if h := vendorKeyHeaders(store.ExportKindLogsDatadog, "k"); h["DD-API-KEY"] != "k" {
		t.Errorf("datadog header = %v", h)
	}
}

func TestVendorPayloads(t *testing.T) {
	at := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	recs := []LogRecord{
		{Time: at, Kind: "app", App: "web", Stream: "stdout", Level: "info", Message: "started"},
		{Time: at, Kind: "app", App: "web", Stream: "stderr", Level: "error", Message: "boom", Fields: json.RawMessage(`{"user":"u1"}`)},
	}
	nr, _ := encodeNewRelicRecords(recs, "plat")
	if !strings.Contains(string(nr), `"service":"web"`) || !strings.Contains(string(nr), `"level":"error"`) || !strings.Contains(string(nr), `"source":"plat"`) {
		t.Errorf("newrelic payload: %s", nr)
	}
	dd, _ := encodeDatadogRecords(recs, "plat")
	if !strings.Contains(string(dd), `"ddsource":"plat"`) || !strings.Contains(string(dd), `"status":"error"`) {
		t.Errorf("datadog payload: %s", dd)
	}
	ax, _ := encodeAxiomRecords(recs, "plat")
	if !strings.Contains(string(ax), `"user":"u1"`) || !strings.Contains(string(ax), `"_time"`) {
		t.Errorf("axiom payload: %s", ax)
	}
	bs, _ := encodeBetterStackRecords(recs, "plat")
	if !strings.Contains(string(bs), `"dt":"2026-10-15 12:00:00.000 UTC"`) {
		t.Errorf("betterstack payload: %s", bs)
	}
}

func TestVendorDestinationNeedsAKeyAndStoresItAsHeader(t *testing.T) {
	db := openDB(t)
	sec := &memSecrets{}
	svc := newTestServiceWith(t, db, telemetry.NewLogBroadcaster(), sec)
	ctx := context.Background()

	_, err := svc.Create(ctx, Input{Name: "nr", Kind: store.ExportKindLogsNewRelic, Enabled: true})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("create without a key = %v, want ErrInvalid", err)
	}
	d, err := svc.Create(ctx, Input{Name: "nr", Kind: store.ExportKindLogsNewRelic, Enabled: true, APIKey: "nrkey", Options: store.ExportOptions{Region: "eu"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if d.Endpoint != "https://log-api.eu.newrelic.com/log/v1" {
		t.Errorf("endpoint = %q", d.Endpoint)
	}
	if len(d.Options.HeaderNames) != 1 || d.Options.HeaderNames[0] != "Api-Key" {
		t.Errorf("header names = %v", d.Options.HeaderNames)
	}

	region := store.ExportOptions{Region: "us"}
	got, err := svc.Update(ctx, d.ID, Patch{Options: &region})
	if err != nil {
		t.Fatalf("update region: %v", err)
	}
	if got.Endpoint != "https://log-api.newrelic.com/log/v1" {
		t.Errorf("endpoint after region change = %q", got.Endpoint)
	}
}

func TestVendorDestinationDeliversWithTheKeyHeader(t *testing.T) {
	db := openDB(t)
	b := telemetry.NewLogBroadcaster()
	svc := newTestService(t, db, b)
	rcv := &sink{}
	srv := httptest.NewServer(rcv)
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// An explicit endpoint overrides the preset, which is how a proxy or a
	// regional host is used.
	if _, err := svc.Create(ctx, Input{
		Name: "dd", Kind: store.ExportKindLogsDatadog, Endpoint: srv.URL, Enabled: true, APIKey: "ddkey",
	}); err != nil {
		t.Fatal(err)
	}
	go svc.Run(ctx)
	waitFor(t, "worker start", func() bool { svc.mu.Lock(); defer svc.mu.Unlock(); return len(svc.workers) == 1 })
	b.Publish(telemetry.LogEntry{ResourceID: "service:web", Stream: "stdout", Timestamp: time.Now(), Message: "hello vendor"})

	waitFor(t, "delivery", func() bool {
		rcv.mu.Lock()
		defer rcv.mu.Unlock()
		return len(rcv.bodies) > 0
	})
	rcv.mu.Lock()
	defer rcv.mu.Unlock()
	if got := rcv.heads[0].Get("DD-API-KEY"); got != "ddkey" {
		t.Errorf("DD-API-KEY = %q", got)
	}
	if !strings.Contains(rcv.bodies[0], "hello vendor") || !strings.Contains(rcv.bodies[0], `"service":"web"`) {
		t.Errorf("body = %s", rcv.bodies[0])
	}
}
