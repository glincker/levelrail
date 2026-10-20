package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/fixit"
)

type staleBackups struct {
	created int
}

func (s *staleBackups) Create(context.Context) (cpbackup.Info, error) {
	s.created++
	return cpbackup.Info{Name: "snap", CreatedAt: time.Now()}, nil
}

func (s *staleBackups) List() ([]cpbackup.Info, error) {
	if s.created > 0 {
		return []cpbackup.Info{{Name: "snap", CreatedAt: time.Now()}}, nil
	}
	return []cpbackup.Info{{Name: "old", CreatedAt: time.Now().Add(-10 * 24 * time.Hour)}}, nil
}

func (s *staleBackups) Open(string) (*os.File, cpbackup.Info, error) {
	return nil, cpbackup.Info{}, os.ErrNotExist
}

func (s *staleBackups) Delete(string) error { return nil }

func (s *staleBackups) Verify(context.Context, string) (cpbackup.VerifyResult, error) {
	return cpbackup.VerifyResult{}, nil
}

func TestStaleBackupFixAppliesAndRechecks(t *testing.T) {
	db := openTestDB(t)
	fake := &staleBackups{}
	rt := NewRouter(discardLogger(), testBrand(), db, WithFixit(db), WithControlPlaneBackups(fake))
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/fixes/backup.control_plane.snapshot/apply", "{}"))
	var res fixit.Result
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &res) != nil || res.Outcome != fixit.OutcomeFixed || fake.created != 1 {
		t.Fatalf("status %d body %s created=%d", rec.Code, rec.Body, fake.created)
	}
}
