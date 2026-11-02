package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/compliance"
)

func newComplianceRouter(t *testing.T) (*Router, *http.Cookie) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db)
	cookie := loginTestSession(t, rt, db)
	signer := compliance.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	svc := compliance.NewService(context.Background(), db, t.TempDir(), signer, rt.ComplianceSecuritySnapshot, rt.ComplianceUserRemover(), logger)
	rt.SetCompliance(svc)
	return rt, cookie
}

func complianceDo(t *testing.T, rt *Router, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
	return rec
}

func TestCompliance_NotConfiguredIs501(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := complianceDo(t, rt, cookie, http.MethodGet, "/api/v1/compliance/access-reviews", "")
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", rec.Code)
	}
}

func TestCompliance_AccessReviewLifecycle(t *testing.T) {
	t.Parallel()
	rt, cookie := newComplianceRouter(t)
	rec := complianceDo(t, rt, cookie, http.MethodPost, "/api/v1/compliance/access-reviews", `{"name":"Q4","due_days":7}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", rec.Code, rec.Body)
	}
	var created struct {
		ID    string `json:"id"`
		Total int    `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.Total == 0 {
		t.Fatalf("created = %+v, %v", created, err)
	}
	if rec := complianceDo(t, rt, cookie, http.MethodPost, "/api/v1/compliance/access-reviews/"+created.ID+"/complete", ""); rec.Code != http.StatusConflict {
		t.Fatalf("complete with undecided rows = %d, want 409", rec.Code)
	}
	var detail struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	rec = complianceDo(t, rt, cookie, http.MethodGet, "/api/v1/compliance/access-reviews/"+created.ID, "")
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	for _, it := range detail.Items {
		path := "/api/v1/compliance/access-reviews/" + created.ID + "/items/" + it.ID
		if rec := complianceDo(t, rt, cookie, http.MethodPut, path, `{"decision":"keep"}`); rec.Code != http.StatusNoContent {
			t.Fatalf("decide status = %d: %s", rec.Code, rec.Body)
		}
	}
	if rec := complianceDo(t, rt, cookie, http.MethodPut, "/api/v1/compliance/access-reviews/"+created.ID+"/items/"+detail.Items[0].ID, `{"decision":"maybe"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad decision status = %d", rec.Code)
	}
	if rec := complianceDo(t, rt, cookie, http.MethodPost, "/api/v1/compliance/access-reviews/"+created.ID+"/complete", ""); rec.Code != http.StatusOK {
		t.Fatalf("complete status = %d: %s", rec.Code, rec.Body)
	}
}

func TestCompliance_EvidencePackDownloadAndVerify(t *testing.T) {
	t.Parallel()
	rt, cookie := newComplianceRouter(t)
	rec := complianceDo(t, rt, cookie, http.MethodPost, "/api/v1/compliance/evidence-packs",
		`{"from":"`+time.Now().AddDate(0, 0, -7).Format("2006-01-02")+`","to":"`+time.Now().Format("2006-01-02")+`"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("create status = %d: %s", rec.Code, rec.Body)
	}
	var pack struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &pack)
	rt.compliance.Wait()
	if rec := complianceDo(t, rt, cookie, http.MethodGet, "/api/v1/compliance/evidence-packs/"+pack.ID+"/download", ""); rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("download = %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	rec = complianceDo(t, rt, cookie, http.MethodPost, "/api/v1/compliance/evidence-packs/"+pack.ID+"/verify", "")
	var rep struct {
		Valid bool `json:"valid"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &rep)
	if rec.Code != http.StatusOK || !rep.Valid {
		t.Fatalf("verify = %d %s", rec.Code, rec.Body)
	}
}
