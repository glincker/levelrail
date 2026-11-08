package api

import (
	"net/http"
	"strings"
	"testing"
)

// applyDetail changes env, applies with the given query and returns the newest event detail.
func applyDetail(t *testing.T, query string) string {
	t.Helper()
	rt, db, cookie := newTimelineRouter(t)
	snapshotFor(t, db, nil)
	putApp(t, rt, cookie, `{"image":"nginx:1","port":80,"env":{"A":"2"}}`)
	if code := tlJSON(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/apply-pending"+query, "", nil); code != http.StatusAccepted {
		t.Fatalf("apply = %d", code)
	}
	var tl timelineResponse
	tlJSON(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/timeline", "", &tl)
	if len(tl.Items) == 0 || tl.Items[0].Title != "Applied pending changes" {
		t.Fatalf("newest event = %+v", tl.Items)
	}
	return tl.Items[0].Detail
}

func TestPendingChangesApplyReasonEnvOnSet(t *testing.T) {
	if got := applyDetail(t, "?reason=env_apply_on_set"); !strings.Contains(got, "apps.env_apply_on_set is on") {
		t.Fatalf("detail %q does not name the reason", got)
	}
	if got := applyDetail(t, ""); strings.Contains(got, "env_apply_on_set") {
		t.Fatalf("detail %q names a reason nobody sent", got)
	}
}

func TestPendingChangesApplyUnknownReasonNotEchoed(t *testing.T) {
	got := applyDetail(t, "?reason=injected%20text")
	if strings.Contains(got, "injected") || strings.Contains(got, "env_apply_on_set") {
		t.Fatalf("detail %q echoed an unknown reason", got)
	}
}
