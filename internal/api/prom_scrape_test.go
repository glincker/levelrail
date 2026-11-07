package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestMetricsScrape_RuntimeSettingTurnsEndpointOff(t *testing.T) {
	t.Parallel()
	rt, db, _ := newExportRouter(t)
	const plaintext = "scrape-fixture-token-value" //nolint:gosec // fake fixture
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_scrape", Name: "prometheus-scrape", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	get := func() int {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		req.Header.Set("Authorization", "Bearer "+plaintext)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	if code := get(); code != http.StatusOK {
		t.Fatalf("enabled by default: status = %d, want 200", code)
	}
	if rt.runtimeSettings == nil {
		t.Skip("runtime settings are not wired in this router")
	}
	if _, err := rt.runtimeSettings.Set(context.Background(), runtimesettings.KeyMetricsScrapeEnabled, []byte(`false`), "test"); err != nil {
		t.Fatal(err)
	}
	if code := get(); code != http.StatusNotFound {
		t.Fatalf("disabled: status = %d, want 404", code)
	}
}
