package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestValidateAppResource(t *testing.T) {
	tests := []struct {
		name    string
		app     appResource
		wantErr bool
	}{
		{name: "valid", app: appResource{Name: "web", Image: "levelrail/web:abc123", Port: 3000}, wantErr: false},
		{name: "missing name", app: appResource{Image: "levelrail/web:abc123", Port: 3000}, wantErr: true},
		{name: "missing image", app: appResource{Name: "web", Port: 3000}, wantErr: true},
		{name: "zero port", app: appResource{Name: "web", Image: "levelrail/web:abc123", Port: 0}, wantErr: true},
		{name: "negative port", app: appResource{Name: "web", Image: "levelrail/web:abc123", Port: -1}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAppResource(tt.app)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAppResource(%+v) error = %v, wantErr %v", tt.app, err, tt.wantErr)
			}
		})
	}
}

// authedRequest builds a request carrying a valid session cookie, so
// each apps-CRUD test can focus on its own scenario instead of repeating
// login setup.
func authedRequest(t *testing.T, cookie *http.Cookie, method, target, body string) *http.Request {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	r.AddCookie(cookie)
	return r
}

func TestAppsRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []struct {
		method string
		target string
	}{
		{http.MethodGet, "/api/v1/apps"},
		{http.MethodPost, "/api/v1/apps"},
		{http.MethodGet, "/api/v1/apps/web"},
		{http.MethodPut, "/api/v1/apps/web"},
		{http.MethodDelete, "/api/v1/apps/web"},
		{http.MethodPost, "/api/v1/apps/web/deploys"},
		{http.MethodGet, "/api/v1/apps/web/deploys"},
	}
	for _, r := range routes {
		t.Run(r.method+" "+r.target, func(t *testing.T) {
			req := httptest.NewRequest(r.method, r.target, nil)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestHandleListApps(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	// Empty first.
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var empty []appResource
	if err := json.Unmarshal(rec.Body.Bytes(), &empty); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected no apps yet, got %d", len(empty))
	}

	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed SaveDesiredService: %v", err)
	}

	rec2 := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec2, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps", ""))
	var got []appResource
	if err := json.Unmarshal(rec2.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Name != "web" {
		t.Fatalf("got %+v, want one app named web", got)
	}
}

func TestHandleCreateApp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	valid := `{"name":"web","image":"levelrail/web:1","port":3000}`

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps", valid))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService after create: %v", err)
	}
	if svc.Image != "levelrail/web:1" || svc.Port != 3000 {
		t.Errorf("saved service = %+v, want image levelrail/web:1 port 3000", svc)
	}

	// Same name again must conflict, not silently overwrite.
	recDup := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recDup, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps", valid))
	if recDup.Code != http.StatusConflict {
		t.Fatalf("duplicate create status = %d, want %d", recDup.Code, http.StatusConflict)
	}

	// Invalid body (missing image): half-succeeded input must not
	// reach the store at all.
	recInvalid := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recInvalid, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps", `{"name":"broken","port":80}`))
	if recInvalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid create status = %d, want %d", recInvalid.Code, http.StatusBadRequest)
	}
	if _, err := db.GetDesiredService(context.Background(), "broken"); err == nil {
		t.Error("invalid create must not have saved a partial app")
	}

	// Malformed JSON body.
	recMalformed := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recMalformed, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps", `{not json`))
	if recMalformed.Code != http.StatusBadRequest {
		t.Fatalf("malformed body status = %d, want %d", recMalformed.Code, http.StatusBadRequest)
	}
}

func TestHandleCreateApp_DomainAlreadyTaken(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	if err := db.SaveDesiredService(context.Background(), store.DesiredService{
		Name: "web", Image: "levelrail/web:1", Port: 3000,
		Domains: []string{"app.example.com"},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps",
		`{"name":"web2","image":"levelrail/web2:1","port":3000,"domains":["app.example.com"]}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "app.example.com") {
		t.Errorf("body = %s, want it to name the conflicting domain", rec.Body.String())
	}

	if _, err := db.GetDesiredService(context.Background(), "web2"); err == nil {
		t.Error("a create rejected for a domain conflict must not have saved a service")
	}
}

func TestHandleUpdateApp_DomainAlreadyTaken(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	if err := db.SaveDesiredService(context.Background(), store.DesiredService{
		Name: "web", Image: "levelrail/web:1", Port: 3000,
		Domains: []string{"app.example.com"},
	}); err != nil {
		t.Fatalf("seed web: %v", err)
	}
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{
		Name: "web2", Image: "levelrail/web2:1", Port: 3000,
	}); err != nil {
		t.Fatalf("seed web2: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web2",
		`{"name":"web2","image":"levelrail/web2:1","port":3000,"domains":["app.example.com"]}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "app.example.com") {
		t.Errorf("body = %s, want it to name the conflicting domain", rec.Body.String())
	}

	web2, err := db.GetDesiredService(context.Background(), "web2")
	if err != nil {
		t.Fatalf("GetDesiredService(web2): %v", err)
	}
	if len(web2.Domains) != 0 {
		t.Errorf("web2.Domains = %v, want unchanged (empty), an update rejected for a domain conflict must not partially apply", web2.Domains)
	}
}

func TestHandleGetApp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got appResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Name != "web" || got.Image != "levelrail/web:1" {
		t.Errorf("got %+v", got)
	}

	recMissing := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recMissing, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/does-not-exist", ""))
	if recMissing.Code != http.StatusNotFound {
		t.Fatalf("status for missing app = %d, want %d", recMissing.Code, http.StatusNotFound)
	}
}

func TestHandleGetApp_DomainsRoundTrip(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	if err := db.SaveDesiredService(context.Background(), store.DesiredService{
		Name: "web", Image: "levelrail/web:1", Port: 3000,
		Domains: []string{"web.example.com", "www.example.com"},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web", ""))
	var got appResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Domains) != 2 || got.Domains[0] != "web.example.com" || got.Domains[1] != "www.example.com" {
		t.Errorf("got.Domains = %v, want [web.example.com www.example.com]", got.Domains)
	}
}

func TestHandleUpdateApp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	update := `{"name":"ignored-by-server","image":"levelrail/web:2","port":4000}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web", update))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService after update: %v", err)
	}
	if svc.Image != "levelrail/web:2" || svc.Port != 4000 {
		t.Errorf("updated service = %+v, want image levelrail/web:2 port 4000", svc)
	}

	// Updating a nonexistent app is 404, not a silent create.
	recMissing := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recMissing, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/ghost", update))
	if recMissing.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recMissing.Code, http.StatusNotFound)
	}

	// Invalid body against an existing app.
	recInvalid := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recInvalid, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web", `{"port":0}`))
	if recInvalid.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recInvalid.Code, http.StatusBadRequest)
	}
}

func TestHandleDeleteApp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	if _, err := db.GetDesiredService(context.Background(), "web"); err == nil {
		t.Error("expected app to be gone from the store after delete")
	}

	recAgain := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recAgain, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web", ""))
	if recAgain.Code != http.StatusNotFound {
		t.Fatalf("deleting an already-deleted app: status = %d, want %d", recAgain.Code, http.StatusNotFound)
	}
}

func TestHandleSetAppNode_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	seedNode(t, db, "node_1", "worker-1")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/node", `{"node_id":"node_1"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got appResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.NodeID != "node_1" {
		t.Errorf("NodeID = %q, want node_1", got.NodeID)
	}

	svc, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if svc.NodeID != "node_1" {
		t.Errorf("stored NodeID = %q, want node_1", svc.NodeID)
	}
}

func TestHandleSetAppNode_EmptyMovesToLocal(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.UpdateServiceNode(ctx, "web", "node_1"); err != nil {
		t.Fatalf("seed placement: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/node", `{"node_id":""}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	svc, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if svc.NodeID != "" {
		t.Errorf("stored NodeID = %q, want empty (moved back to local)", svc.NodeID)
	}
}

func TestHandleSetAppNode_UnknownNode_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/node", `{"node_id":"nonexistent"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	svc, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if svc.NodeID != "" {
		t.Errorf("stored NodeID = %q, want unchanged (empty): a rejected request must not partially apply", svc.NodeID)
	}
}

func TestHandleSetAppNode_UnknownApp_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/ghost/node", `{"node_id":""}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestHandleSetAppNode_CordonedNode_Rejected is TASKS.md 3.7's cordon
// enforcement point: a cordoned node must refuse new placements while
// leaving whatever's already running there untouched (store.Node.
// Schedulable's own doc comment).
func TestHandleSetAppNode_CordonedNode_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	seedNode(t, db, "node_1", "worker-1")
	if err := db.SetNodeSchedulable(ctx, "node_1", false); err != nil {
		t.Fatalf("cordon node: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/node", `{"node_id":"node_1"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	svc, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if svc.NodeID != "" {
		t.Errorf("stored NodeID = %q, want unchanged (empty): a rejected request must not partially apply", svc.NodeID)
	}
}

func TestHandleSetAppNode_PlainWriteToken_Forbidden(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/web/node", strings.NewReader(`{"node_id":""}`))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: placement is fleet-level, a plain write token must not reach it", rec.Code, http.StatusForbidden)
	}
}
