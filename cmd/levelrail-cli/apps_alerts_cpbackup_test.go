package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRun_AppsAlertsCreate_ControlPlaneBackupStale(t *testing.T) {
	var gotBody createAlertRuleRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(alertRuleResource{ID: "alr_5", Name: gotBody.Name, Kind: gotBody.Kind, Enabled: gotBody.Enabled})
	}))
	defer srv.Close()

	mustRunAppsAlertsCreate(t, srv.URL, "--name", "cp-stale", "--kind", "control_plane_backup_stale")
	if gotBody.Kind != "control_plane_backup_stale" {
		t.Errorf("request body = %+v, want a control_plane_backup_stale rule", gotBody)
	}
}
