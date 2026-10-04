package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestAppStreamRoutes_RequireAuth(t *testing.T) {
	rt, _, _ := newTimelineRouter(t)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/apps/web/streams", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleCreateAppStream_Success(t *testing.T) {
	rt, db, cookie := newTimelineRouter(t)

	before, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}

	var got appStreamResource
	body := `{"container_port":5433,"host_port":15432}`
	if code := tlJSON(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/streams", body, &got); code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", code, http.StatusCreated)
	}
	if got.ID == "" || !strings.HasPrefix(got.ID, "stream_") {
		t.Errorf("ID = %q, want a stream_ prefix", got.ID)
	}
	if got.App != "web" || got.ContainerPort != 5433 || got.HostPort != 15432 || got.Protocol != "tcp" {
		t.Errorf("created stream = %+v, want app=web container_port=5433 host_port=15432 protocol=tcp", got)
	}

	// The owning service's RestartNonce was bumped so the next container
	// recreation picks up the new published port: see
	// handleCreateAppStream's own doc comment.
	after, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	if after.RestartNonce == before.RestartNonce {
		t.Errorf("RestartNonce unchanged after creating a stream, want it bumped")
	}

	listed, err := db.ListAppStreamsForService(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != got.ID {
		t.Errorf("ListAppStreamsForService() = %+v, want exactly the created stream", listed)
	}
}

func TestHandleCreateAppStream_AppNotFound(t *testing.T) {
	rt, _, cookie := newTimelineRouter(t)

	body := `{"container_port":5433,"host_port":15432}`
	if code := tlJSON(t, rt, cookie, http.MethodPost, "/api/v1/apps/missing/streams", body, nil); code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", code, http.StatusNotFound)
	}
}

func TestHandleCreateAppStream_InvalidPorts(t *testing.T) {
	rt, _, cookie := newTimelineRouter(t)

	tests := []string{
		`{"container_port":0,"host_port":15432}`,
		`{"container_port":5433,"host_port":0}`,
		`{"container_port":5433,"host_port":15432,"protocol":"udp"}`,
	}
	for _, body := range tests {
		if code := tlJSON(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/streams", body, nil); code != http.StatusBadRequest {
			t.Errorf("body %q: status = %d, want %d", body, code, http.StatusBadRequest)
		}
	}
}

func TestHandleCreateAppStream_DuplicateHostPortConflict(t *testing.T) {
	rt, _, cookie := newTimelineRouter(t)

	if err := rt.apps.SaveDesiredService(context.Background(), store.DesiredService{Name: "redis", Image: "redis:7", Port: 6379}); err != nil {
		t.Fatal(err)
	}

	body := `{"container_port":5433,"host_port":15432}`
	if code := tlJSON(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/streams", body, nil); code != http.StatusCreated {
		t.Fatalf("first create status = %d, want %d", code, http.StatusCreated)
	}

	collide := `{"container_port":6380,"host_port":15432}`
	if code := tlJSON(t, rt, cookie, http.MethodPost, "/api/v1/apps/redis/streams", collide, nil); code != http.StatusConflict {
		t.Fatalf("colliding host port status = %d, want %d", code, http.StatusConflict)
	}
}

func TestHandleListAppStreams(t *testing.T) {
	rt, _, cookie := newTimelineRouter(t)

	body := `{"container_port":5433,"host_port":15432}`
	if code := tlJSON(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/streams", body, nil); code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", code, http.StatusCreated)
	}

	var got []appStreamResource
	if code := tlJSON(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/streams", "", &got); code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", code, http.StatusOK)
	}
	if len(got) != 1 || got[0].HostPort != 15432 {
		t.Fatalf("list = %+v, want exactly one stream on host port 15432", got)
	}
}

func TestHandleDeleteAppStream(t *testing.T) {
	rt, db, cookie := newTimelineRouter(t)

	var created appStreamResource
	body := `{"container_port":5433,"host_port":15432}`
	if code := tlJSON(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/streams", body, &created); code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", code, http.StatusCreated)
	}

	if code := tlJSON(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web/streams/"+created.ID, "", nil); code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", code, http.StatusNoContent)
	}

	listed, err := db.ListAppStreamsForService(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Errorf("ListAppStreamsForService() after delete = %+v, want none", listed)
	}

	if code := tlJSON(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web/streams/"+created.ID, "", nil); code != http.StatusNotFound {
		t.Fatalf("second delete status = %d, want %d", code, http.StatusNotFound)
	}
}

// TestHandleDeleteAppStream_WrongApp404s confirms a stream ID that
// exists but belongs to a different app is refused as not found, not
// silently deleted: the {name} path segment scopes the delete, the same
// per-resource ownership check AbilityWriteSensitive's own appResourceFromPath
// is meant to enforce.
func TestHandleDeleteAppStream_WrongApp404s(t *testing.T) {
	rt, _, cookie := newTimelineRouter(t)

	if err := rt.apps.SaveDesiredService(context.Background(), store.DesiredService{Name: "redis", Image: "redis:7", Port: 6379}); err != nil {
		t.Fatal(err)
	}

	var created appStreamResource
	body := `{"container_port":5433,"host_port":15432}`
	if code := tlJSON(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/streams", body, &created); code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", code, http.StatusCreated)
	}

	if code := tlJSON(t, rt, cookie, http.MethodDelete, "/api/v1/apps/redis/streams/"+created.ID, "", nil); code != http.StatusNotFound {
		t.Fatalf("delete via wrong app status = %d, want %d", code, http.StatusNotFound)
	}
}
