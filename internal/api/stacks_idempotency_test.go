package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStackApplyIdempotencyKey(t *testing.T) {
	rt, _, cookie := stackFixture(t)
	body := stackBody(t, stackTestDocs("ghcr.io/acme/web:1.2.3"), nil)
	do := func(key string) *httptest.ResponseRecorder {
		req := authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/shop-prod/apply", body)
		req.Header.Set(headerIdempotencyKey, key)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec
	}
	first, second := do("k1"), do("k1")
	if first.Code != http.StatusOK || second.Code != http.StatusOK || second.Header().Get(headerIdempotentReplayed) != "true" {
		t.Fatalf("first %d, second %d replayed %q", first.Code, second.Code, second.Header().Get(headerIdempotentReplayed))
	}
	if first.Body.String() != second.Body.String() {
		t.Fatal("replay must return the stored result")
	}
	var hist struct {
		Revisions []stackRevisionSummary `json:"revisions"`
	}
	iacCall(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/stacks/shop-prod/revisions", ""), &hist)
	if len(hist.Revisions) != 1 {
		t.Fatalf("revisions = %d, want 1", len(hist.Revisions))
	}
}
