package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestInboxArchiveAndState(t *testing.T) {
	rt, _, cookie := newInboxRouter(t)
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/inbox/archive", `{"ids":["deploy:d1"]}`); rec.Code != http.StatusNoContent {
		t.Fatalf("archive = %d %s", rec.Code, rec.Body.String())
	}
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/inbox/read", `{"ids":["approval:a1"]}`); rec.Code != http.StatusNoContent {
		t.Fatalf("read = %d", rec.Code)
	}
	var st inboxStateResource
	rec := apiDo(t, rt, cookie, http.MethodGet, "/api/v1/inbox/state", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if len(st.ArchivedIDs) != 1 || st.ArchivedIDs[0] != "deploy:d1" || len(st.ReadIDs) != 1 {
		t.Fatalf("state = %s", rec.Body.String())
	}
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/inbox/archive", `{"ids":[]}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty archive = %d, want 400", rec.Code)
	}
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/inbox/archive", `{"ids":["deploy:d1"],"restore":true}`); rec.Code != http.StatusNoContent {
		t.Fatalf("restore = %d", rec.Code)
	}
}
