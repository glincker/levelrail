package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/dbprovision"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

type noDockerVolumes struct{ t *testing.T }

func (n noDockerVolumes) ListNamedVolumes(context.Context) ([]docker.NamedVolume, error) {
	n.t.Error("create must not call Docker")
	return nil, nil
}

func (n noDockerVolumes) RemoveVolume(context.Context, string) error {
	n.t.Error("create must not call Docker")
	return nil
}

func doDB(t *testing.T, rt *Router, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
	return rec
}

func TestCreateDatabase_StoresEverythingPendingWithoutDocker(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.orphanedVolumes = noDockerVolumes{t}
	cookie := loginTestSession(t, rt, db)

	body := `{"name":"main","engine":"redis","version":"7","existing_volume":"reuse","resources":{"memory_bytes":268435456}}`
	rec := doDB(t, rt, cookie, http.MethodPost, "/api/v1/databases", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", rec.Code, rec.Body.String())
	}
	saved, err := db.GetDesiredDatabase(context.Background(), "main")
	if err != nil {
		t.Fatal(err)
	}
	if saved.ExistingVolume != "reuse" || saved.Resources == nil || saved.Resources.MemoryBytes != 268435456 || saved.VolumeResolved {
		t.Fatalf("stored record = %+v, want every setting kept and the volume unresolved", saved)
	}
	got := getDatabaseResource(t, rt, cookie, "main")
	if got.ProvisionState != provisionStatePending {
		t.Fatalf("provision_state = %q, want pending", got.ProvisionState)
	}
	var list []databaseListResource
	_ = json.Unmarshal(doDB(t, rt, cookie, http.MethodGet, "/api/v1/databases", "").Body.Bytes(), &list)
	if len(list) != 1 || list[0].ProvisionState != provisionStatePending {
		t.Fatalf("list = %+v, want one pending record", list)
	}
}

func TestCreateDatabase_RejectsBadExistingVolumeChoice(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := doDB(t, rt, cookie, http.MethodPost, "/api/v1/databases", `{"name":"main","engine":"redis","version":"7","existing_volume":"maybe"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestPendingDatabase_FailureKeepsRecordEditRetryDiscard(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	mustCreateDatabase(t, rt, cookie, `{"name":"main","engine":"redis","version":"7","resources":{"memory_bytes":134217728}}`)

	tr := dbprovision.NewRecorder(db).Open(ctx, "main")
	tr.Fail(dbprovision.StepCreateVolume, &dbprovision.LeftoverVolumeError{Name: "db-main-data", SizeBytes: 2048})

	got := getDatabaseResource(t, rt, cookie, "main")
	if got.ProvisionState != provisionStateFailed || got.ProvisionReason == "" || got.Resources == nil {
		t.Fatalf("detail = %+v, want failed with the reason and settings kept", got)
	}

	rec := doDB(t, rt, cookie, http.MethodPut, "/api/v1/databases/main/pending-settings", `{"version":"7","resources":{"memory_bytes":536870912}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("edit status = %d body=%s", rec.Code, rec.Body.String())
	}
	if d, _ := db.GetDesiredDatabase(ctx, "main"); d.Resources == nil || d.Resources.MemoryBytes != 536870912 {
		t.Fatalf("edited record = %+v, want the new size stored", d)
	}

	if rec := doDB(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/provision/retry", `{"fix":"remove_leftover_volume"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("remove without confirm = %d, want 400", rec.Code)
	}
	if rec := doDB(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/provision/retry", `{"fix":"reuse_existing_volume"}`); rec.Code != http.StatusAccepted {
		t.Fatalf("retry reuse = %d body=%s", rec.Code, rec.Body.String())
	}
	if d, _ := db.GetDesiredDatabase(ctx, "main"); d.ExistingVolume != "reuse" {
		t.Fatalf("existing_volume = %q, want the stored reuse choice", d.ExistingVolume)
	}

	if rec := doDB(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/provision/discard", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("discard without confirm = %d, want 400", rec.Code)
	}
	if rec := doDB(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/provision/discard", `{"confirm":true}`); rec.Code != http.StatusNoContent {
		t.Fatalf("discard = %d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := db.GetDesiredDatabase(ctx, "main"); err == nil {
		t.Fatal("record survived the discard")
	}
}

func TestPendingDatabase_CreatedDatabaseCannotBeEditedOrDiscarded(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	mustCreateDatabase(t, rt, cookie, `{"name":"main","engine":"redis","version":"7"}`)
	dbprovision.NewRecorder(db).Open(ctx, "main").Finish()

	if rec := doDB(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/provision/discard", `{"confirm":true}`); rec.Code != http.StatusConflict {
		t.Fatalf("discard of a created database = %d, want 409", rec.Code)
	}
	if rec := doDB(t, rt, cookie, http.MethodPut, "/api/v1/databases/main/pending-settings", `{"version":"7"}`); rec.Code != http.StatusConflict {
		t.Fatalf("edit of a created database = %d, want 409", rec.Code)
	}
	if got := getDatabaseResource(t, rt, cookie, "main"); got.ProvisionState != "" {
		t.Fatalf("provision_state = %q, want empty once created", got.ProvisionState)
	}
	_ = store.EngineRedis
}
