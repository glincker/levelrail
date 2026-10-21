package api

import (
	"net/http"
	"testing"
)

func TestInboxDeleteRejectsDerivedItems(t *testing.T) {
	rt, _, cookie := newInboxRouter(t)
	rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/inbox/delete", `{"ids":["deploy:d1"]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("delete derived = %d %s", rec.Code, rec.Body.String())
	}
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/inbox/delete", `{"ids":[]}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("delete empty = %d", rec.Code)
	}
}

func TestInboxClearNeedsConfirmCountToDelete(t *testing.T) {
	rt, _, cookie := newInboxRouter(t)
	body := `{"action":"delete","scope":"archived","confirm_count":3}`
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/inbox/clear", body); rec.Code != http.StatusBadRequest {
		t.Fatalf("mismatched confirm_count = %d %s", rec.Code, rec.Body.String())
	}
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/inbox/clear", `{"action":"delete","scope":"archived","dry_run":true}`); rec.Code != http.StatusOK {
		t.Fatalf("dry run = %d %s", rec.Code, rec.Body.String())
	}
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/inbox/clear", `{"action":"delete","scope":"archived","confirm_count":0}`); rec.Code != http.StatusOK {
		t.Fatalf("empty clear = %d %s", rec.Code, rec.Body.String())
	}
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/inbox/clear", `{"action":"archive","scope":"archived"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("archive archived = %d", rec.Code)
	}
}
