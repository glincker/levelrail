package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func listAuditLog(t *testing.T, rt *Router, cookie *http.Cookie) []auditLogEntryResource {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/audit-log", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /audit-log status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []auditLogEntryResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode audit log: %v", err)
	}
	return got
}

// TestAudit_WriteRequestRecorded covers (a) from the task spec: a real
// AbilityWrite request (app creation) must produce a matching audit
// entry with the right actor and status.
func TestAudit_WriteRequestRecorded(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps", `{"name":"web","image":"levelrail/web:1","port":3000}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create app status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	entries := listAuditLog(t, rt, cookie)
	var found *auditLogEntryResource
	for i := range entries {
		if entries[i].Method == http.MethodPost && entries[i].Path == "/api/v1/apps" {
			found = &entries[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("no audit entry recorded for POST /api/v1/apps, got %+v", entries)
	}
	if found.StatusCode != http.StatusCreated {
		t.Errorf("StatusCode = %d, want %d", found.StatusCode, http.StatusCreated)
	}
	if found.ActorType != "session" {
		t.Errorf("ActorType = %q, want %q", found.ActorType, "session")
	}
	if found.ActorName != testAdminUsername {
		t.Errorf("ActorName = %q, want %q", found.ActorName, testAdminUsername)
	}
	if found.Ability != AbilityWrite {
		t.Errorf("Ability = %q, want %q", found.Ability, AbilityWrite)
	}
}

// TestAudit_ReadRequestNotRecorded covers (b) from the task spec: an
// AbilityRead request must never produce an audit entry.
func TestAudit_ReadRequestNotRecorded(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("list apps status = %d, want %d", rec.Code, http.StatusOK)
	}

	entries := listAuditLog(t, rt, cookie)
	for _, e := range entries {
		if e.Method == http.MethodGet && e.Path == "/api/v1/apps" {
			t.Fatalf("GET /api/v1/apps (AbilityRead) must not be audited, found %+v", e)
		}
	}
}

// TestAudit_FailedRequestRecorded proves the audit log isn't just a
// success log: a real non-2xx response from a real handler (malformed
// create-app body) still gets recorded with its real status code.
func TestAudit_FailedRequestRecorded(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps", `{not json`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed create app status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	entries := listAuditLog(t, rt, cookie)
	var found *auditLogEntryResource
	for i := range entries {
		if entries[i].Method == http.MethodPost && entries[i].Path == "/api/v1/apps" {
			found = &entries[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("no audit entry recorded for the failed POST /api/v1/apps, got %+v", entries)
	}
	if found.StatusCode != http.StatusBadRequest {
		t.Errorf("StatusCode = %d, want %d (a failed request must still be audited with its real status)", found.StatusCode, http.StatusBadRequest)
	}
}

// TestAudit_TokenActorRecorded covers a bearer-token caller: ActorType
// must be "token" and ActorName must be the token's own stored Name,
// not a placeholder.
func TestAudit_TokenActorRecorded(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write", Name: "ci deploy bot", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", strings.NewReader(`{"name":"worker","image":"levelrail/worker:1","port":4000}`))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create app status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	entries := listAuditLog(t, rt, cookie)
	var found *auditLogEntryResource
	for i := range entries {
		if entries[i].ActorType == "token" {
			found = &entries[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("no token-actor audit entry recorded, got %+v", entries)
	}
	if found.ActorID != "tok_write" {
		t.Errorf("ActorID = %q, want %q", found.ActorID, "tok_write")
	}
	if found.ActorName != "ci deploy bot" {
		t.Errorf("ActorName = %q, want the token's own stored Name, got %q", found.ActorName, found.ActorName)
	}
}

// TestAuditLogRoute_RequiresRoot proves GET /api/v1/audit-log is gated
// at AbilityRoot, not merely AbilityRead: a token scoped to read (or any
// non-root ability) must be forbidden.
func TestAuditLogRoute_RequiresRoot(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()

	const plaintext = "read-only-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_ro", Name: "read only", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-log", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d: a read-scoped token must not reach the audit log", rec.Code, http.StatusForbidden)
	}
}

// TestAuditLogRoute_Pagination exercises ?limit and ?before against a
// real store, through the real handler.
func TestAuditLogRoute_Pagination(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	// Seed three write-ability requests worth of audit history directly,
	// so pagination has deterministic, well-separated timestamps to work
	// with rather than depending on real wall-clock request timing.
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, id := range []string{"aud_seed_1", "aud_seed_2", "aud_seed_3"} {
		e := store.AuditEntry{
			ID:         id,
			ActorType:  "session",
			ActorID:    "user_seed",
			ActorName:  "seed",
			Ability:    AbilityWrite,
			Method:     http.MethodPost,
			Path:       "/api/v1/apps",
			StatusCode: http.StatusCreated,
			RemoteAddr: "127.0.0.1",
			CreatedAt:  store.FormatAuditTime(base.Add(time.Duration(i) * time.Hour)),
		}
		if err := db.SaveAuditEntry(ctx, e); err != nil {
			t.Fatalf("seed audit entry %s: %v", id, err)
		}
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/audit-log?limit=2", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var page1 []auditLogEntryResource
	if err := json.Unmarshal(rec.Body.Bytes(), &page1); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("len(page1) = %d, want 2", len(page1))
	}
	if page1[0].ID != "aud_seed_3" || page1[1].ID != "aud_seed_2" {
		t.Fatalf("page1 order = [%s %s], want [aud_seed_3 aud_seed_2]", page1[0].ID, page1[1].ID)
	}

	before := page1[len(page1)-1].CreatedAt
	rec2 := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec2, authedRequest(t, cookie, http.MethodGet, "/api/v1/audit-log?before="+url.QueryEscape(before), ""))
	if rec2.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec2.Code, http.StatusOK, rec2.Body.String())
	}
	var page2 []auditLogEntryResource
	if err := json.Unmarshal(rec2.Body.Bytes(), &page2); err != nil {
		t.Fatalf("decode: %v", err)
	}
	found := false
	for _, e := range page2 {
		if e.ID == "aud_seed_1" {
			found = true
		}
		if e.ID == "aud_seed_3" || e.ID == "aud_seed_2" {
			t.Errorf("page2 must not repeat entries already seen in page1, got %s", e.ID)
		}
	}
	if !found {
		t.Errorf("page2 missing aud_seed_1, got %+v", page2)
	}
}

func TestHandleListAuditLog_InvalidLimit(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/audit-log?limit=not-a-number", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleListAuditLog_InvalidBefore(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/audit-log?before=not-a-timestamp", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
