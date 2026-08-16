package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/spec"
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
		{name: "empty strategy falls through to store default", app: appResource{Name: "web", Image: "levelrail/web:abc123", Port: 3000, Strategy: ""}, wantErr: false},
		{name: "recreate strategy valid", app: appResource{Name: "web", Image: "levelrail/web:abc123", Port: 3000, Strategy: "recreate"}, wantErr: false},
		{name: "blue-green strategy valid", app: appResource{Name: "web", Image: "levelrail/web:abc123", Port: 3000, Strategy: "blue-green"}, wantErr: false},
		{name: "rolling strategy valid (real spec constant, reconciler-unsupported but not an API validation concern)", app: appResource{Name: "web", Image: "levelrail/web:abc123", Port: 3000, Strategy: "rolling"}, wantErr: false},
		{name: "unknown strategy rejected", app: appResource{Name: "web", Image: "levelrail/web:abc123", Port: 3000, Strategy: "bogus"}, wantErr: true},
		{name: "zero replicas falls through to store default", app: appResource{Name: "web", Image: "levelrail/web:abc123", Port: 3000, Replicas: 0}, wantErr: false},
		{name: "positive replicas valid", app: appResource{Name: "web", Image: "levelrail/web:abc123", Port: 3000, Replicas: 3}, wantErr: false},
		{name: "negative replicas rejected", app: appResource{Name: "web", Image: "levelrail/web:abc123", Port: 3000, Replicas: -1}, wantErr: true},
		{name: "custom labels valid", app: appResource{Name: "web", Image: "levelrail/web:abc123", Port: 3000, Labels: map[string]string{"team": "platform"}}, wantErr: false},
		{name: "reserved-prefix label rejected", app: appResource{Name: "web", Image: "levelrail/web:abc123", Port: 3000, Labels: map[string]string{spec.ReservedLabelPrefix + "managed": "true"}}, wantErr: true},
		{name: "empty label key rejected", app: appResource{Name: "web", Image: "levelrail/web:abc123", Port: 3000, Labels: map[string]string{"": "x"}}, wantErr: true},
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

// TestHandleListApps_Status covers the batched status field
// (appListResource): one app with a True condition, one with a False
// condition, and one with no conditions recorded at all, to prove each
// row gets the right independent category rather than every row
// collapsing to whatever the first controller's status happened to be
// (the kind of bug a batched-but-mis-keyed implementation would produce).
func TestHandleListApps_Status(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	for _, name := range []string{"healthy-app", "broken-app", "pending-app"} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{
			Name: name, Image: "levelrail/" + name + ":1", Port: 3000,
		}); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	if err := db.UpsertConditions(ctx, "application/healthy-app", []reconcile.Condition{
		{Type: "Ready", Status: reconcile.ConditionTrue, Reason: "Running"},
	}); err != nil {
		t.Fatalf("upsert healthy-app conditions: %v", err)
	}
	if err := db.UpsertConditions(ctx, "application/broken-app", []reconcile.Condition{
		{Type: "Ready", Status: reconcile.ConditionFalse, Reason: "CrashLoop"},
	}); err != nil {
		t.Fatalf("upsert broken-app conditions: %v", err)
	}
	// pending-app deliberately gets no UpsertConditions call at all.

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []appListResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d apps, want 3", len(got))
	}

	byName := map[string]appListResource{}
	for _, a := range got {
		byName[a.Name] = a
	}

	if s := byName["healthy-app"].Status; s.Label != "Healthy" || s.Variant != "success" {
		t.Errorf("healthy-app status = %+v, want Healthy/success", s)
	}
	if s := byName["broken-app"].Status; s.Label != "Attention needed" || s.Variant != "destructive" {
		t.Errorf("broken-app status = %+v, want Attention needed/destructive", s)
	}
	if s := byName["pending-app"].Status; s.Label != "No status yet" || s.Variant != "muted" {
		t.Errorf("pending-app status = %+v, want No status yet/muted", s)
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

// TestHandleCreateApp_StrategyAndReplicas covers both an explicit value
// and the omitted-field case, which must resolve to store.DefaultDeployStrategy/
// store.DefaultReplicas exactly like a service saved without these fields
// always has (store.SaveDesiredService's own defense, not something this
// handler re-implements).
func TestHandleCreateApp_StrategyAndReplicas(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	explicit := `{"name":"web","image":"levelrail/web:1","port":3000,"strategy":"recreate","replicas":3}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps", explicit))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService after create: %v", err)
	}
	if svc.Strategy != "recreate" || svc.Replicas != 3 {
		t.Errorf("saved service = %+v, want strategy=recreate replicas=3", svc)
	}
	var got appResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Strategy != "recreate" || got.Replicas != 3 {
		t.Errorf("response = %+v, want strategy=recreate replicas=3", got)
	}

	// Omitted strategy/replicas must resolve to the store's own defaults,
	// the same as any other app.yaml-less create today.
	implicit := `{"name":"web-defaults","image":"levelrail/web:1","port":3000}`
	recImplicit := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recImplicit, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps", implicit))
	if recImplicit.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", recImplicit.Code, http.StatusCreated, recImplicit.Body.String())
	}
	svcDefault, err := db.GetDesiredService(context.Background(), "web-defaults")
	if err != nil {
		t.Fatalf("GetDesiredService after implicit create: %v", err)
	}
	if svcDefault.Strategy != store.DefaultDeployStrategy || svcDefault.Replicas != store.DefaultReplicas {
		t.Errorf("saved service = %+v, want strategy=%s replicas=%d", svcDefault, store.DefaultDeployStrategy, store.DefaultReplicas)
	}
}

func TestHandleCreateApp_Labels(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"name":"web","image":"levelrail/web:1","port":3000,"labels":{"team":"platform","tier":"frontend"}}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService after create: %v", err)
	}
	if svc.Labels["team"] != "platform" || svc.Labels["tier"] != "frontend" || len(svc.Labels) != 2 {
		t.Errorf("saved labels = %+v, want map[team:platform tier:frontend]", svc.Labels)
	}
	var got appResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Labels["team"] != "platform" || got.Labels["tier"] != "frontend" {
		t.Errorf("response labels = %+v, want map[team:platform tier:frontend]", got.Labels)
	}
}

// TestHandleCreateApp_ReservedLabelPrefix_Rejected proves the collision-
// prevention rule is enforced through the general create endpoint too,
// not only through app.yaml parsing: a custom label under
// spec.ReservedLabelPrefix must never be accepted, since it's this
// platform's own reserved namespace.
func TestHandleCreateApp_ReservedLabelPrefix_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"name":"web","image":"levelrail/web:1","port":3000,"labels":{"` + spec.ReservedLabelPrefix + `managed":"true"}}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if _, err := db.GetDesiredService(context.Background(), "web"); err == nil {
		t.Error("a reserved-prefix label must not have saved a service")
	}
}

// TestHandleCreateApp_InvalidStrategy_Rejected: a typo'd strategy value
// must be a 400, not silently coerced to a default or saved verbatim.
func TestHandleCreateApp_InvalidStrategy_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps",
		`{"name":"web","image":"levelrail/web:1","port":3000,"strategy":"canary"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if _, err := db.GetDesiredService(context.Background(), "web"); err == nil {
		t.Error("an invalid strategy must not have saved a service")
	}
}

func TestHandleUpdateApp_StrategyAndReplicas(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	update := `{"name":"web","image":"levelrail/web:1","port":3000,"strategy":"blue-green","replicas":5}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web", update))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService after update: %v", err)
	}
	if svc.Strategy != "blue-green" || svc.Replicas != 5 {
		t.Errorf("updated service = %+v, want strategy=blue-green replicas=5", svc)
	}
}

func TestHandleGetApp_StrategyAndReplicasRoundTrip(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	if err := db.SaveDesiredService(context.Background(), store.DesiredService{
		Name: "web", Image: "levelrail/web:1", Port: 3000,
		Strategy: "recreate", Replicas: 2,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web", ""))
	var got appResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Strategy != "recreate" || got.Replicas != 2 {
		t.Errorf("got = %+v, want strategy=recreate replicas=2", got)
	}

	// A service saved without an explicit strategy/replicas still comes
	// back with the store's resolved defaults, never empty/zero: see
	// store.DesiredService's own doc comment on why these two fields are
	// always resolved.
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "bare", Image: "levelrail/bare:1", Port: 3000}); err != nil {
		t.Fatalf("seed bare: %v", err)
	}
	recBare := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recBare, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/bare", ""))
	var gotBare appResource
	if err := json.Unmarshal(recBare.Body.Bytes(), &gotBare); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if gotBare.Strategy != store.DefaultDeployStrategy || gotBare.Replicas != store.DefaultReplicas {
		t.Errorf("got = %+v, want strategy=%s replicas=%d", gotBare, store.DefaultDeployStrategy, store.DefaultReplicas)
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

func TestHandleRestartApp_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	before, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/restart", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	after, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if after.RestartNonce == before.RestartNonce {
		t.Errorf("RestartNonce unchanged by the restart endpoint: %q", after.RestartNonce)
	}
	if after.Image != before.Image {
		t.Errorf("Image changed by a restart: %q, want unchanged %q", after.Image, before.Image)
	}
}

func TestHandleRestartApp_UnknownApp_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/nonexistent/restart", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleRestartApp_PlainWriteToken_Forbidden(t *testing.T) {
	// Restart is a deploy-adjacent action (it forces a real container
	// recreation), the same ability boundary POST .../deploys and
	// POST .../builds already draw (AbilityDeploy, not AbilityWrite):
	// see router.go's registration of this route for why.
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

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/restart", strings.NewReader(""))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not be able to force a container recreation", rec.Code, http.StatusForbidden)
	}
}

func TestHandleStopApp_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/stop", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if !got.Suspended {
		t.Error("Suspended = false, want true after POST .../stop")
	}
}

func TestHandleStopApp_UnknownApp_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/nonexistent/stop", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleStartApp_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.UpdateServiceSuspended(ctx, "web", true); err != nil {
		t.Fatalf("seed suspended: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/start", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if got.Suspended {
		t.Error("Suspended = true, want false after POST .../start")
	}
}

func TestHandleStopApp_PlainWriteToken_Forbidden(t *testing.T) {
	// Same AbilityDeploy boundary as restart (see router.go's
	// registration of this route).
	rt, db := newTestRouter(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	const plaintext = "write-scoped-token-stop" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write_stop", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/stop", strings.NewReader(""))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not be able to stop an app", rec.Code, http.StatusForbidden)
	}
}
