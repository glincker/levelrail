package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

func TestHandleCreateApp_ReplicaCap(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if _, err := rt.runtimeSettings.Set(t.Context(), runtimesettings.KeyAppsMaxReplicasPerService, json.RawMessage("5"), "test"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		replicas string
		want     int
	}{{"cap1", "5", http.StatusCreated}, {"over1", "6", http.StatusBadRequest}} {
		body := `{"name":"` + tc.name + `","image":"levelrail/web:1","port":3000,"replicas":` + tc.replicas + `}`
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps", body))
		if rec.Code != tc.want {
			t.Errorf("replicas %s: status = %d, want %d (%s)", tc.replicas, rec.Code, tc.want, rec.Body.String())
		}
	}
}
