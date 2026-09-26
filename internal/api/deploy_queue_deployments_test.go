package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func listDeployments(t *testing.T, rt *Router, cookie *http.Cookie, query string) map[string]deploymentResource {
	t.Helper()
	rec := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/deployments"+query, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("deployments = %d %s", rec.Code, rec.Body.String())
	}
	var out deploymentListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	byID := map[string]deploymentResource{}
	for _, d := range out.Items {
		byID[d.ID] = d
	}
	return byID
}

func TestDeploymentsListCarriesQueueCancelAndSupersedeFields(t *testing.T) {
	rt, db, cookie, gb := newQueueRouter(t)
	if err := db.SetServiceCancelSuperseded(t.Context(), "web", true); err != nil {
		t.Fatal(err)
	}
	running := postBuild(t, rt, cookie, "web", "main")
	gb.awaitStart(t)
	old := postBuild(t, rt, cookie, "web", "feature")
	newer := postBuild(t, rt, cookie, "web", "feature")
	third := postBuild(t, rt, cookie, "web", "docs")
	if code, _ := cancelDeploy(t, rt, cookie, "web", third.ID); code != http.StatusOK {
		t.Fatalf("cancel = %d", code)
	}

	all := listDeployments(t, rt, cookie, "?status=queued&status=canceled&status=superseded")
	q := all[newer.ID]
	if q.Status != "queued" || q.QueuePosition == nil || *q.QueuePosition != 1 || q.WaitReason == nil || *q.WaitReason != "waiting for #"+running.ID || q.BlockedBy == nil || q.QueuedAt == nil {
		t.Fatalf("queued item = %+v", q)
	}
	s := all[old.ID]
	if s.Status != "superseded" || s.SupersededBy == nil || *s.SupersededBy != newer.ID {
		t.Fatalf("superseded item = %+v", s)
	}
	c := all[third.ID]
	if c.Status != "canceled" || c.CanceledBy == nil || c.Reason == "" {
		t.Fatalf("canceled item = %+v", c)
	}
	if _, ok := all[running.ID]; ok {
		t.Fatal("a running deploy must not match the queued, canceled or superseded filters")
	}
	gb.release <- struct{}{}
	gb.awaitStart(t)
	gb.release <- struct{}{}
}
