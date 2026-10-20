package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/platformimport/fakecoolify"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestAppImportVerifyBuildAndFailure(t *testing.T) {
	h := newAppImportHarness(t)
	ctx := context.Background()
	builder := h.rt.builder.(*fakeBuilder)
	v := h.view(h.call(http.MethodPost, appImportBase+"/sessions", h.connectBody("")))
	id := v.ID
	plan := `{"selected":["` + v.item("web").SourceID + `"],"mappings":[{"from":"` + fakecoolify.PGUUID + `","to":"pg-main"}]}`
	if rec := h.call(http.MethodPut, appImportBase+"/sessions/"+id+"/plan", plan); rec.Code != http.StatusOK {
		t.Fatalf("plan: %d", rec.Code)
	}
	if rec := h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/stage", `{}`); rec.Code != http.StatusOK {
		t.Fatalf("stage: %d %s", rec.Code, rec.Body.String())
	}
	state := func() store.AppImportItem {
		items, _ := h.db.ListAppImportItems(ctx, id)
		for _, it := range items {
			if it.SourceName == "web" {
				return it
			}
		}
		return store.AppImportItem{}
	}

	builder.err = errors.New("build exploded")
	if rec := h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/verify", `{}`); rec.Code != http.StatusAccepted {
		t.Fatalf("verify: %d %s", rec.Code, rec.Body.String())
	}
	waitFor(t, 10*time.Second, func() bool { return state().State == store.AppImportVerifyFailed })
	if svc, _ := h.db.GetDesiredService(ctx, "web"); !svc.Suspended {
		t.Error("a failed verification must leave the app stopped")
	}
	if !strings.Contains(state().Reason, "deploy history") {
		t.Errorf("reason lacks the logs pointer: %q", state().Reason)
	}

	builder.err = nil
	h.ready("web")
	waitFor(t, 5*time.Second, func() bool { return !h.rt.appImportLive.isRunning(id) })
	if rec := h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/verify", `{}`); rec.Code != http.StatusAccepted {
		t.Fatalf("verify again: %d %s", rec.Code, rec.Body.String())
	}
	waitFor(t, 10*time.Second, func() bool { return state().State == store.AppImportVerified })
	if builder.calls == 0 {
		t.Error("the repository app was never built")
	}
}
