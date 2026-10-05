package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

func TestHandleCreateDatabase_LeftoverVolume(t *testing.T) {
	data := docker.NamedVolume{Name: "db-main-data", SizeBytes: 2048}
	certs := docker.NamedVolume{Name: "db-main-certs"}
	tests := []struct {
		name        string
		volumes     []docker.NamedVolume
		choice      string
		wantStatus  int
		wantRemoved []string
		wantCode    string
	}{
		{name: "no leftover creates", wantStatus: http.StatusCreated},
		{name: "other database volume ignored", volumes: []docker.NamedVolume{{Name: "db-other-data"}}, wantStatus: http.StatusCreated},
		{name: "leftover without choice is refused", volumes: []docker.NamedVolume{data}, wantStatus: http.StatusConflict, wantCode: errCodeExistingVolume},
		{name: "reuse keeps volumes", volumes: []docker.NamedVolume{data}, choice: "reuse", wantStatus: http.StatusCreated},
		{name: "discard removes data and certs", volumes: []docker.NamedVolume{data, certs}, choice: "discard", wantStatus: http.StatusCreated, wantRemoved: []string{"db-main-data", "db-main-certs"}},
		{name: "discard refuses a mounted volume", volumes: []docker.NamedVolume{{Name: "db-main-data", Mounted: true}}, choice: "discard", wantStatus: http.StatusConflict},
		{name: "unknown choice", volumes: []docker.NamedVolume{data}, choice: "maybe", wantStatus: http.StatusBadRequest},
		{name: "certs only leftover is not a data conflict", volumes: []docker.NamedVolume{certs}, wantStatus: http.StatusCreated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeOrphanedVolumeManager{volumes: tt.volumes}
			rt, db := newTestRouterWithOrphanedVolumeManager(t, fake)
			cookie := loginTestSession(t, rt, db)

			body := `{"name":"main","engine":"redis","version":"7"`
			if tt.choice != "" {
				body += `,"existing_volume":"` + tt.choice + `"`
			}
			body += `}`
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases", body))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantCode != "" {
				var got existingVolumeConflict
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("decode conflict: %v", err)
				}
				if got.Code != tt.wantCode || len(got.Volumes) != 1 || got.Volumes[0].Name != "db-main-data" {
					t.Errorf("conflict = %+v", got)
				}
			}
			if !reflect.DeepEqual(fake.removed, tt.wantRemoved) {
				t.Errorf("removed = %v, want %v", fake.removed, tt.wantRemoved)
			}
		})
	}
}
