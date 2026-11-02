package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDataCopyFeed_StaleAndDismissal(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	raw := `dump of the source failed: docker: exec [sh -c export SRC_PASSWORD=hunter2 exec pg_dump] in abc: exited 1: pg_dump: error: connection to server at "72.61.8.70", port 5432 failed: timeout expired`

	for _, name := range []string{"geokit", "glinrv2"} {
		if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: name, Engine: store.EnginePostgres, Version: "16"}); err != nil {
			t.Fatal(err)
		}
		started := time.Now().Add(-2 * time.Hour)
		if err := db.ClaimDatabaseDataImport(ctx, name, "72.61.8.70", 5432, "src", started, time.Hour); err != nil {
			t.Fatal(err)
		}
		if err := db.FinishDatabaseDataImport(ctx, name, store.DataImportFailed, raw, 0, 0, "", started.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
	}

	feed := func() map[string]attention.Item {
		out := map[string]attention.Item{}
		for _, it := range fetchFeed(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/attention/feed", "")) {
			if it.Kind == attention.KindDataCopy {
				out[it.Subject] = it
			}
		}
		return out
	}
	got := feed()
	if len(got) != 2 {
		t.Fatalf("data copy items = %d, want 2", len(got))
	}
	it := got["geokit"]
	if strings.Contains(it.Detail, "exec [sh") || strings.Contains(it.Params["technical"], "hunter2") {
		t.Errorf("item not humanised or leaks secret: %+v", it)
	}

	dismiss := func(key string) int {
		return dismissItem(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/attention/dismiss", `{"item_key":"`+key+`"}`))
	}
	if code := dismiss("data_copy:geokit:1"); code != http.StatusNotFound {
		t.Errorf("dismiss stale attempt = %d, want 404", code)
	}
	if code := dismiss(it.ID); code != http.StatusNoContent {
		t.Fatalf("dismiss = %d, want 204", code)
	}
	after := feed()
	if _, still := after["geokit"]; still || len(after) != 1 {
		t.Errorf("after dismissal items = %v, want only glinrv2", after)
	}

	retryAt := time.Now().Add(-time.Minute)
	if err := db.ClaimDatabaseDataImport(ctx, "geokit", "72.61.8.70", 5432, "src", retryAt, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishDatabaseDataImport(ctx, "geokit", store.DataImportFailed, raw, 0, 0, "", retryAt.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, back := feed()["geokit"]; !back {
		t.Error("a new failed attempt must reappear after an earlier one was dismissed")
	}
}
