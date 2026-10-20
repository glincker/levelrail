package api

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuditLogRoute_SearchAndFailed(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	do := func(method, path, body string) {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		_ = rec
	}
	do(http.MethodPost, "/api/v1/apps", `{"name":"web","image":"levelrail/web:1","port":3000}`)
	do(http.MethodPost, "/api/v1/apps", `{not json`)

	get := func(query string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/audit-log"+query, ""))
		return rec
	}
	decode := func(rec *httptest.ResponseRecorder) []auditLogEntryResource {
		t.Helper()
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var out []auditLogEntryResource
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return out
	}

	failed := decode(get("?status=failed"))
	if len(failed) != 1 || failed[0].StatusCode != http.StatusBadRequest {
		t.Fatalf("status=failed = %+v, want exactly the 400 entry", failed)
	}

	if got := decode(get("?q=POST")); len(got) < 2 {
		t.Errorf("q=POST returned %d entries, want at least 2", len(got))
	}
	if got := decode(get("?q=no-such-needle")); len(got) != 0 {
		t.Errorf("q=no-such-needle = %+v, want none", got)
	}
	if got := decode(get("?q=post&status=failed")); len(got) != 1 {
		t.Errorf("q+status = %+v, want 1", got)
	}

	csvRec := get("?format=csv&status=failed")
	rows, err := csv.NewReader(strings.NewReader(csvRec.Body.String())).ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	if len(rows) != 2 || rows[1][7] != "400" {
		t.Errorf("csv rows = %v, want header + one 400 row", rows)
	}

	if rec := get("?status=bogus"); rec.Code != http.StatusBadRequest {
		t.Errorf("status=bogus code = %d, want 400", rec.Code)
	}
}
