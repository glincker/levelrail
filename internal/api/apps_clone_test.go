package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// TestHandleCloneApp_Success covers the fields that do carry over
// verbatim: image, port, env, secret_env (names only, see the
// dedicated secret-value test below), resources, health, strategy,
// replicas.
func TestHandleCloneApp_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	source := store.DesiredService{
		Name:  "web",
		Image: "levelrail/web:1",
		Port:  3000,
		Env:   map[string]string{"LOG_LEVEL": "info"},
		Resources: &store.ServiceResources{
			MemoryBytes: 512 * 1024 * 1024,
			NanoCPUs:    500_000_000,
		},
		Health: &store.ServiceHealth{
			Readiness: &store.ServiceProbe{Path: "/healthz"},
		},
		Strategy: "recreate",
		Replicas: 3,
	}
	if err := db.SaveDesiredService(ctx, source); err != nil {
		t.Fatalf("seed source: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/clone", `{"new_name":"web-copy"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got appResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Name != "web-copy" {
		t.Errorf("response name = %q, want web-copy", got.Name)
	}

	clone, err := db.GetDesiredService(ctx, "web-copy")
	if err != nil {
		t.Fatalf("GetDesiredService after clone: %v", err)
	}
	if clone.Image != source.Image || clone.Port != source.Port {
		t.Errorf("clone image/port = %q/%d, want %q/%d", clone.Image, clone.Port, source.Image, source.Port)
	}
	if clone.Env["LOG_LEVEL"] != "info" {
		t.Errorf("clone env = %+v, want LOG_LEVEL=info carried over", clone.Env)
	}
	if clone.Resources == nil || clone.Resources.MemoryBytes != source.Resources.MemoryBytes {
		t.Errorf("clone resources = %+v, want memory_bytes carried over from source", clone.Resources)
	}
	if clone.Health == nil || clone.Health.Readiness == nil || clone.Health.Readiness.Path != "/healthz" {
		t.Errorf("clone health = %+v, want readiness path carried over from source", clone.Health)
	}
	if clone.Strategy != "recreate" || clone.Replicas != 3 {
		t.Errorf("clone strategy/replicas = %q/%d, want recreate/3", clone.Strategy, clone.Replicas)
	}

	// Source must be entirely untouched by cloning it.
	stillSource, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService source after clone: %v", err)
	}
	if stillSource.Image != source.Image {
		t.Errorf("source mutated by clone: image = %q, want %q", stillSource.Image, source.Image)
	}
}

// TestHandleCloneApp_NameConflict covers both a clone target that
// collides with an unrelated existing app and the self-clone case
// (new_name equal to the source's own name), which must be rejected the
// same way rather than silently overwriting the source in place.
func TestHandleCloneApp_NameConflict(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed web: %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "worker", Image: "levelrail/worker:1", Port: 4000}); err != nil {
		t.Fatalf("seed worker: %v", err)
	}

	recExisting := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recExisting, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/clone", `{"new_name":"worker"}`))
	if recExisting.Code != http.StatusConflict {
		t.Fatalf("clone onto existing name: status = %d, want %d", recExisting.Code, http.StatusConflict)
	}

	recSelf := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recSelf, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/clone", `{"new_name":"web"}`))
	if recSelf.Code != http.StatusConflict {
		t.Fatalf("self-clone: status = %d, want %d", recSelf.Code, http.StatusConflict)
	}
}

func TestHandleCloneApp_SourceNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/ghost/clone", `{"new_name":"ghost-copy"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleCloneApp_MissingNewName_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/clone", `{}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleCloneApp_MalformedBody(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/clone", `{not json`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestHandleCloneApp_DomainsNotCarriedOver: cloning must not attempt to
// claim the source's domains for the new service (see handleCloneApp's
// own doc comment for why: the source still owns them, so copying them
// verbatim would always collide with store.ErrDomainTaken). The clone
// must succeed domainless, not fail.
func TestHandleCloneApp_DomainsNotCarriedOver(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{
		Name: "web", Image: "levelrail/web:1", Port: 3000,
		Domains: []string{"web.example.com"},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/clone", `{"new_name":"web-copy"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	clone, err := db.GetDesiredService(ctx, "web-copy")
	if err != nil {
		t.Fatalf("GetDesiredService after clone: %v", err)
	}
	if len(clone.Domains) != 0 {
		t.Errorf("clone domains = %v, want none carried over from source", clone.Domains)
	}
}

// TestHandleCloneApp_SecretEnvNamesCarryOverWithoutValues covers this
// endpoint's explicit secrets decision: the declared secret-backed env
// var *names* carry over (so the clone's shape and required-secret
// validation match the source), but the underlying encrypted value does
// not, since internal/secrets scopes a DEK per service name and this
// endpoint never touches the secrets manager at all.
func TestHandleCloneApp_SecretEnvNamesCarryOverWithoutValues(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{
		Name: "web", Image: "levelrail/web:1", Port: 3000,
		SecretEnv: []string{"API_KEY"},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/clone", `{"new_name":"web-copy"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	clone, err := db.GetDesiredService(ctx, "web-copy")
	if err != nil {
		t.Fatalf("GetDesiredService after clone: %v", err)
	}
	if len(clone.SecretEnv) != 1 || clone.SecretEnv[0] != "API_KEY" {
		t.Errorf("clone secret_env = %v, want [API_KEY] name carried over", clone.SecretEnv)
	}

	// No DEK or value should exist yet for the clone's own service name:
	// this endpoint must never touch internal/secrets at all, so nothing
	// is set under the clone's name until the operator explicitly does
	// so via PUT /api/v1/apps/{name}/secrets/{key}.
	if _, err := db.GetServiceDEK(ctx, "web-copy"); !errors.Is(err, store.ErrServiceDEKNotFound) {
		t.Errorf("GetServiceDEK(web-copy) error = %v, want ErrServiceDEKNotFound: clone must not have generated a DEK or copied a secret value", err)
	}
}

// TestHandleCloneApp_ProjectInherited_NodeNot covers this endpoint's
// other explicit judgment call: project assignment (a pure
// organizational label) is inherited automatically, node placement (a
// real capacity/infrastructure decision) is not, matching
// handleCreateApp's own established boundary for the latter.
func TestHandleCloneApp_ProjectInherited_NodeNot(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "Staging", CreatedAt: time.Now().Format(time.RFC3339)}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if err := db.SaveNode(ctx, store.Node{ID: "node_1", Name: "box-1", Status: store.NodeStatusOnline}); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed web: %v", err)
	}
	if err := db.UpdateServiceProject(ctx, "web", "proj_1"); err != nil {
		t.Fatalf("assign source project: %v", err)
	}
	if err := db.UpdateServiceNode(ctx, "web", "node_1"); err != nil {
		t.Fatalf("assign source node: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/clone", `{"new_name":"web-copy"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	clone, err := db.GetDesiredService(ctx, "web-copy")
	if err != nil {
		t.Fatalf("GetDesiredService after clone: %v", err)
	}
	if clone.ProjectID != "proj_1" {
		t.Errorf("clone project_id = %q, want proj_1 inherited from source", clone.ProjectID)
	}
	if clone.NodeID != "" {
		t.Errorf("clone node_id = %q, want empty (local node), placement must never be inherited silently", clone.NodeID)
	}

	var got appResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ProjectID != "proj_1" {
		t.Errorf("response project_id = %q, want proj_1", got.ProjectID)
	}
	if got.NodeID != "" {
		t.Errorf("response node_id = %q, want empty", got.NodeID)
	}
}

func TestHandleCloneApp_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/clone", strings.NewReader(`{"new_name":"web-copy"}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
