// TestResourceAdvisor_HTTP_SuggestApplyDismiss drives the resource advisor
// over a real HTTP server with a real store and telemetry database and no
// container engine: list suggestions, preview then confirm one, dismiss and
// restore another, and check a read-only token cannot change anything.
package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const (
	e2eAdvisorAdminUsername = "e2e-advisor-admin"
	e2eAdvisorAdminPassword = "e2e-advisor-correct-horse" //nolint:gosec // test fixture credential, not a real secret
	e2eAdvisorMiB           = 1024 * 1024
)

type e2eAdvisorFinding struct {
	ID       string `json:"id"`
	State    string `json:"state"`
	CanApply bool   `json:"can_apply"`
}

type e2eAdvisorApply struct {
	Applied         bool `json:"applied"`
	ConfirmRequired bool `json:"confirm_required"`
	After           struct {
		MemoryBytes int64 `json:"memory_bytes"`
	} `json:"after"`
}

func TestResourceAdvisor_HTTP_SuggestApplyDismiss(t *testing.T) {
	ctx := context.Background()
	experimental.Set(experimental.All()...)
	t.Cleanup(func() { experimental.Set(experimental.All()...) })
	db := openLiveStore(t)
	tdb, err := telemetry.Open(ctx, filepath.Join(t.TempDir(), "telemetry.db"))
	if err != nil {
		t.Fatalf("telemetry.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = tdb.Close() })

	// Two unlimited databases with three days of steady hourly usage.
	now := time.Now().UTC().Truncate(time.Second)
	var samples []telemetry.Sample
	for name, mib := range map[string]float64{"e2eadvisora": 200, "e2eadvisorb": 60} {
		if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: name, Engine: store.EnginePostgres, Version: "16"}); err != nil {
			t.Fatalf("SaveDesiredDatabase(%s) error = %v", name, err)
		}
		for i := 0; i < 72; i++ {
			ts := now.Add(-time.Duration(i) * time.Hour)
			samples = append(samples,
				telemetry.Sample{ResourceID: "database:" + name, Metric: "memory_usage_bytes", Timestamp: ts, Value: mib * e2eAdvisorMiB},
				telemetry.Sample{ResourceID: "database:" + name, Metric: "cpu_percent", Timestamp: ts, Value: 3},
			)
		}
	}
	if err := tdb.WriteSamples(ctx, samples); err != nil {
		t.Fatalf("WriteSamples() error = %v", err)
	}

	b := &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}
	router := api.NewRouter(discardTestLogger(), b, db, api.WithTelemetryQuerier(telemetry.NewLocalFederator(tdb)))
	ts := newE2ETestServer(t, router)
	if err := api.BootstrapAdmin(ctx, db, e2eAdvisorAdminUsername, e2eAdvisorAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	admin := loginE2EClient(t, ts.URL, e2eAdvisorAdminUsername, e2eAdvisorAdminPassword)
	base := ts.URL + "/api/v1/resource-advisor/"

	list := func(query string) map[string]e2eAdvisorFinding {
		t.Helper()
		status, raw := requestJSON(t, admin, http.MethodGet, base+"findings"+query, "")
		if status != http.StatusOK {
			t.Fatalf("list findings: status = %d, body = %.300s", status, raw)
		}
		var out struct {
			Findings []e2eAdvisorFinding `json:"findings"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("decode findings: %v", err)
		}
		byID := map[string]e2eAdvisorFinding{}
		for _, f := range out.Findings {
			byID[f.ID] = f
		}
		return byID
	}
	applyID := "memory_limit_missing:database:e2eadvisora"
	dismissID := "memory_limit_missing:database:e2eadvisorb"

	found := list("")
	for _, id := range []string{applyID, dismissID} {
		if f, ok := found[id]; !ok || !f.CanApply {
			t.Fatalf("finding %s missing or not appliable: %+v", id, f)
		}
	}

	// Apply: the first call only previews, the confirmed call saves the limit.
	status, raw := requestJSON(t, admin, http.MethodPost, base+"findings/"+applyID+"/apply", "")
	var preview e2eAdvisorApply
	if err := json.Unmarshal(raw, &preview); err != nil || status != http.StatusOK || !preview.ConfirmRequired || preview.Applied || preview.After.MemoryBytes < 200*e2eAdvisorMiB {
		t.Fatalf("preview: status = %d, body = %.300s, err = %v", status, raw, err)
	}
	if d, _ := db.GetDesiredDatabase(ctx, "e2eadvisora"); d.Resources != nil && d.Resources.MemoryBytes != 0 {
		t.Fatal("preview changed the database")
	}
	status, raw = requestJSON(t, admin, http.MethodPost, base+"findings/"+applyID+"/apply", `{"confirm":true}`)
	var done e2eAdvisorApply
	if err := json.Unmarshal(raw, &done); err != nil || status != http.StatusOK || !done.Applied {
		t.Fatalf("confirm: status = %d, body = %.300s, err = %v", status, raw, err)
	}
	d, err := db.GetDesiredDatabase(ctx, "e2eadvisora")
	if err != nil || d.Resources == nil || d.Resources.MemoryBytes != preview.After.MemoryBytes {
		t.Fatalf("limit not saved: %+v, err = %v", d, err)
	}
	if _, ok := list("")[applyID]; ok {
		t.Fatal("applied finding is still listed")
	}

	// A read-only token can read but not change findings.
	readToken := createE2EToken(t, admin, ts.URL, "e2e-advisor-read", `["read"]`)
	req, err := http.NewRequest(http.MethodPost, base+"findings/"+dismissID+"/dismiss", nil) //nolint:noctx // test helper
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+readToken)
	resp, err := (&http.Client{Timeout: e2eHTTPTimeout}).Do(req)
	if err != nil {
		t.Fatalf("read-only dismiss: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("read-only dismiss = %d, want 403", resp.StatusCode)
	}

	// Dismiss hides the finding, restore brings it back.
	if status, raw := requestJSON(t, admin, http.MethodPost, base+"findings/"+dismissID+"/dismiss", ""); status != http.StatusOK {
		t.Fatalf("dismiss: status = %d, body = %.300s", status, raw)
	}
	if _, ok := list("")[dismissID]; ok {
		t.Fatal("dismissed finding is still listed")
	}
	if f, ok := list("?include=hidden")[dismissID]; !ok || f.State != store.AdvisorFindingDismissed {
		t.Fatalf("hidden list = %+v", f)
	}
	if status, raw := requestJSON(t, admin, http.MethodPost, base+"findings/"+dismissID+"/restore", ""); status != http.StatusOK {
		t.Fatalf("restore: status = %d, body = %.300s", status, raw)
	}
	if _, ok := list("")[dismissID]; !ok {
		t.Fatal("restored finding is missing")
	}
}
