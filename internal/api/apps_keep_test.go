package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleKeepTrialApp(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		abilities []string
		trial     bool
		target    string
		repeat    bool
		wantCode  int
	}{
		{name: "anonymous", target: "web", trial: true, wantCode: http.StatusUnauthorized},
		{name: "read token cannot keep", abilities: []string{AbilityRead}, target: "web", trial: true, wantCode: http.StatusForbidden},
		{name: "deploy token cannot keep", abilities: []string{AbilityDeploy}, target: "web", trial: true, wantCode: http.StatusForbidden},
		{name: "write token keeps", abilities: []string{AbilityWrite}, target: "web", trial: true, wantCode: http.StatusOK},
		{name: "keep is idempotent", abilities: []string{AbilityWrite}, target: "web", trial: true, repeat: true, wantCode: http.StatusOK},
		{name: "not a trial is a conflict", abilities: []string{AbilityWrite}, target: "web", wantCode: http.StatusConflict},
		{name: "missing app", abilities: []string{AbilityRoot}, target: "nope", wantCode: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			ctx := context.Background()
			if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:1", Port: 80, IsTrial: tt.trial}); err != nil {
				t.Fatalf("seed: %v", err)
			}
			calls := 0
			call := func() *httptest.ResponseRecorder {
				calls++
				req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/"+tt.target+"/keep", nil)
				if tt.abilities != nil {
					req.Header.Set("Authorization", "Bearer "+seedMatrixToken(t, db, "tok-keep-"+strconv.Itoa(calls), tt.abilities))
				}
				rec := httptest.NewRecorder()
				rt.Handler().ServeHTTP(rec, req)
				return rec
			}
			rec := call()
			if tt.repeat {
				rec = call()
			}
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.wantCode, rec.Body.String())
			}
			if rec.Code != http.StatusOK {
				if svc, _ := db.GetDesiredService(ctx, "web"); svc != nil && svc.IsTrial != tt.trial {
					t.Fatalf("a refused keep changed the trial flag")
				}
				return
			}
			var got keepTrialResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.AlreadyKept != tt.repeat || got.KeptAt.IsZero() || (!tt.repeat && len(got.Kept) != 1) {
				t.Fatalf("response = %+v", got)
			}
			if svc, _ := db.GetDesiredService(ctx, "web"); svc == nil || svc.IsTrial {
				t.Fatal("app is still flagged as a trial after keep")
			}
		})
	}
}
