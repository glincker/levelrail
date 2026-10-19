package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIdempotencyKey_ReplaysAndDetectsConflict(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	body := `{"name":"idem","image":"levelrail/web:1","port":3000}`

	do := func(key, payload string) *httptest.ResponseRecorder {
		req := authedRequest(t, cookie, http.MethodPost, "/api/v1/apps", payload)
		if key != "" {
			req.Header.Set(headerIdempotencyKey, key)
		}
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec
	}

	first := do("k1", body)
	if first.Code != http.StatusCreated {
		t.Fatalf("first status = %d, body = %s", first.Code, first.Body.String())
	}
	replay := do("k1", body)
	if replay.Code != http.StatusCreated || replay.Header().Get(headerIdempotentReplayed) != "true" {
		t.Fatalf("replay status = %d replayed = %q, want 201 true", replay.Code, replay.Header().Get(headerIdempotentReplayed))
	}
	if replay.Body.String() != first.Body.String() {
		t.Errorf("replay body differs from first response")
	}
	if got := do("k1", `{"name":"other","image":"levelrail/web:1","port":3000}`); got.Code != http.StatusUnprocessableEntity {
		t.Errorf("changed body status = %d, want 422", got.Code)
	}
	if got := do("", body); got.Code != http.StatusConflict {
		t.Errorf("no key duplicate status = %d, want 409", got.Code)
	}
}

func TestPageSlice(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	tests := []struct {
		query string
		want  []int
	}{
		{"", []int{1, 2, 3, 4, 5}},
		{"limit=2", []int{1, 2}},
		{"limit=2&offset=3", []int{4, 5}},
		{"offset=99", []int{}},
	}
	for _, tc := range tests {
		rec := httptest.NewRecorder()
		got := pageQuery(rec, httptest.NewRequest(http.MethodGet, "/x?"+tc.query, nil), items)
		if len(got) != len(tc.want) {
			t.Errorf("%q: got %v, want %v", tc.query, got, tc.want)
		}
		if rec.Header().Get("X-Total-Count") != "5" {
			t.Errorf("%q: X-Total-Count = %q", tc.query, rec.Header().Get("X-Total-Count"))
		}
	}
}
