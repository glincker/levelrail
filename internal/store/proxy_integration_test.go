package store

import (
	"context"
	"testing"
	"time"
)

func TestProxyIntegrationSettings_DefaultsRoundTripAndValidation(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	got, err := db.GetProxyIntegrationSettings(ctx)
	if err != nil {
		t.Fatalf("GetProxyIntegrationSettings() error = %v", err)
	}
	if got.Mode != ProxyIntegrationOff || got.Enabled() {
		t.Fatalf("default mode = %q, want off", got.Mode)
	}

	want := ProxyIntegrationSettings{Mode: ProxyIntegrationTraefikFile, DynamicDir: "/data/coolify/proxy/dynamic", EntrypointHTTP: "http", EntrypointHTTPS: "https", CertResolver: "letsencrypt", UpstreamHost: "host.docker.internal"}
	if err := db.UpdateProxyIntegrationSettings(ctx, want); err != nil {
		t.Fatalf("UpdateProxyIntegrationSettings() error = %v", err)
	}
	if got, _ = db.GetProxyIntegrationSettings(ctx); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if err := db.UpdateProxyIntegrationSettings(ctx, ProxyIntegrationSettings{Mode: "nginx"}); err == nil {
		t.Error("unknown mode: expected an error")
	}
}

func TestProxyRouteStatus_Lifecycle(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	at := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

	if err := db.MarkProxyRouteFailed(ctx, "a.example.com", "app:web", "directory missing"); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkProxyRouteWritten(ctx, "a.example.com", "app:web", "x-managed-a.example.com.yaml", at, true); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveProxyRouteVerification(ctx, ProxyRouteStatus{Domain: "a.example.com", ProxyLoaded: ProxyLoadedYes, Reachable: true, StatusCode: 200, CertIssuer: "R11", CertTrusted: true, CertLetsEncrypt: true, VerifiedAt: at}); err != nil {
		t.Fatal(err)
	}
	// An unchanged rewrite keeps the verification and the first write time.
	if err := db.MarkProxyRouteWritten(ctx, "a.example.com", "app:web", "x-managed-a.example.com.yaml", at.Add(time.Hour), false); err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListProxyRouteStatus(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatalf("ListProxyRouteStatus() = %v, %v", rows, err)
	}
	r := rows[0]
	if !r.Written || r.LastError != "" || !r.Reachable || !r.CertLetsEncrypt || !r.VerifiedAt.Equal(at) || !r.WrittenAt.Equal(at) {
		t.Errorf("unexpected row %+v", r)
	}
	// A changed file clears the verification so it is probed again.
	if err := db.MarkProxyRouteWritten(ctx, "a.example.com", "app:web", "x-managed-a.example.com.yaml", at.Add(2*time.Hour), true); err != nil {
		t.Fatal(err)
	}
	rows, _ = db.ListProxyRouteStatus(ctx)
	if !rows[0].VerifiedAt.IsZero() || !rows[0].WrittenAt.Equal(at.Add(2*time.Hour)) {
		t.Errorf("changed write did not reset verification: %+v", rows[0])
	}
	if err := db.DeleteProxyRouteStatus(ctx, "a.example.com"); err != nil {
		t.Fatal(err)
	}
	if rows, _ = db.ListProxyRouteStatus(ctx); len(rows) != 0 {
		t.Errorf("rows after delete = %v", rows)
	}
}
