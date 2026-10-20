package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeNetworkShareSecretsSetter mirrors fakeRegistryCredentialSecretsSetter's
// shape for the single-method NetworkShareSecretsSetter surface.
type fakeNetworkShareSecretsSetter struct {
	err   error
	calls []struct{ serviceName, envKey, plaintext string }
}

func (f *fakeNetworkShareSecretsSetter) SetValue(_ context.Context, serviceName, envKey, plaintext string) error {
	f.calls = append(f.calls, struct{ serviceName, envKey, plaintext string }{serviceName, envKey, plaintext})
	return f.err
}

func newTestRouterWithNetworkShareSecrets(t *testing.T, secrets NetworkShareSecretsSetter) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithNetworkShareSecrets(secrets)), db
}

func TestValidateNetworkShareFields(t *testing.T) {
	tests := []struct {
		name            string
		protocol        string
		username        string
		password        string
		requirePassword bool
		wantErr         bool
	}{
		{name: "valid nfs, no credentials", protocol: store.NetworkShareProtocolNFS, wantErr: false},
		{name: "valid cifs with password required", protocol: store.NetworkShareProtocolCIFS, username: "u", password: "p", requirePassword: true, wantErr: false},
		{name: "cifs missing username", protocol: store.NetworkShareProtocolCIFS, password: "p", requirePassword: true, wantErr: true},
		{name: "cifs missing password when required", protocol: store.NetworkShareProtocolCIFS, username: "u", requirePassword: true, wantErr: true},
		{name: "cifs missing password when not required", protocol: store.NetworkShareProtocolCIFS, username: "u", requirePassword: false, wantErr: false},
		{name: "unsupported protocol", protocol: "ftp", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNetworkShareFields("nas", tt.protocol, "nas.lan", "/export", tt.username, tt.password, tt.requirePassword)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateNetworkShareFields() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNetworkShareRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/network-shares"},
		{http.MethodPost, "/api/v1/network-shares"},
		{http.MethodGet, "/api/v1/network-shares/ns_x"},
		{http.MethodPut, "/api/v1/network-shares/ns_x"},
		{http.MethodDelete, "/api/v1/network-shares/ns_x"},
		{http.MethodPost, "/api/v1/network-shares/ns_x/test"},
	})
}

func TestHandleCreateNetworkShare_NFS_NoSetterConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithNetworkShareSecrets
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"name":"media-nas","protocol":"nfs","host":"nas.lan","remote_path":"/export/media"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/network-shares", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s: an nfs share needs no secrets manager", rec.Code, http.StatusCreated, rec.Body.String())
	}
}

func TestHandleCreateNetworkShare_CIFS_NoSetterConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithNetworkShareSecrets
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"name":"backup-nas","protocol":"cifs","host":"nas.lan","remote_path":"/backups","username":"u","password":"p"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/network-shares", body))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleCreateNetworkShare_InvalidRequest(t *testing.T) {
	rt, db := newTestRouterWithNetworkShareSecrets(t, &fakeNetworkShareSecretsSetter{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/network-shares", `{"name":""}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

// The half-succeeded case: a CIFS share's password write fails, so the
// store row must never be saved, the same ordering
// TestHandleCreateRegistryCredential_SecretSetFails already verifies for
// its own sibling resource.
func TestHandleCreateNetworkShare_CIFS_SecretSetFails(t *testing.T) {
	setter := &fakeNetworkShareSecretsSetter{err: errors.New("master key not configured")}
	rt, db := newTestRouterWithNetworkShareSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"name":"backup-nas","protocol":"cifs","host":"nas.lan","remote_path":"/backups","username":"u","password":"p"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/network-shares", body))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	got, err := db.ListNetworkShares(context.Background())
	if err != nil {
		t.Fatalf("ListNetworkShares() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListNetworkShares() = %+v, want no row saved when the secret write fails first", got)
	}
}

func TestHandleCreateNetworkShare_DuplicateName_Conflict(t *testing.T) {
	setter := &fakeNetworkShareSecretsSetter{}
	rt, db := newTestRouterWithNetworkShareSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)

	body := `{"name":"media-nas","protocol":"nfs","host":"nas.lan","remote_path":"/export/media"}`
	firstRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(firstRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/network-shares", body))
	if firstRec.Code != http.StatusCreated {
		t.Fatalf("first create status = %d, want %d", firstRec.Code, http.StatusCreated)
	}

	secondRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(secondRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/network-shares", body))
	if secondRec.Code != http.StatusConflict {
		t.Fatalf("second create status = %d, want %d", secondRec.Code, http.StatusConflict)
	}
}

func TestHandleGetNetworkShare_NotFound(t *testing.T) {
	rt, db := newTestRouterWithNetworkShareSecrets(t, &fakeNetworkShareSecretsSetter{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/network-shares/ns_missing", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleUpdateNetworkShare_Success(t *testing.T) {
	setter := &fakeNetworkShareSecretsSetter{}
	rt, db := newTestRouterWithNetworkShareSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)

	createRec := httptest.NewRecorder()
	createBody := `{"name":"media-nas","protocol":"nfs","host":"nas.lan","remote_path":"/export/media"}`
	rt.Handler().ServeHTTP(createRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/network-shares", createBody))
	var created networkShareResource
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	setter.calls = nil

	updateRec := httptest.NewRecorder()
	updateBody := `{"name":"media-nas-renamed","protocol":"nfs","host":"nas2.lan","remote_path":"/export/media2"}`
	rt.Handler().ServeHTTP(updateRec, authedRequest(t, cookie, http.MethodPut, "/api/v1/network-shares/"+created.ID, updateBody))
	if updateRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", updateRec.Code, http.StatusOK, updateRec.Body.String())
	}

	var got networkShareResource
	if err := json.NewDecoder(updateRec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Name != "media-nas-renamed" || got.Host != "nas2.lan" {
		t.Errorf("response = %+v, want the updated fields", got)
	}
	if len(setter.calls) != 0 {
		t.Errorf("SetValue calls = %d, want 0 when the request carries no password", len(setter.calls))
	}
}

func TestHandleDeleteNetworkShare_Success(t *testing.T) {
	setter := &fakeNetworkShareSecretsSetter{}
	rt, db := newTestRouterWithNetworkShareSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)

	createRec := httptest.NewRecorder()
	createBody := `{"name":"media-nas","protocol":"nfs","host":"nas.lan","remote_path":"/export/media"}`
	rt.Handler().ServeHTTP(createRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/network-shares", createBody))
	var created networkShareResource
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	deleteRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(deleteRec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/network-shares/"+created.ID, ""))
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", deleteRec.Code, http.StatusNoContent)
	}

	getRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(getRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/network-shares/"+created.ID, ""))
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("status after delete = %d, want %d", getRec.Code, http.StatusNotFound)
	}
}

func TestHandleTestNetworkShare_NotFound(t *testing.T) {
	rt, db := newTestRouterWithNetworkShareSecrets(t, &fakeNetworkShareSecretsSetter{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/network-shares/ns_missing/test", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleTestNetworkShare_DialsHostPort(t *testing.T) {
	setter := &fakeNetworkShareSecretsSetter{}
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))

	var dialedNetwork, dialedAddr string
	fakeDialer := func(_ context.Context, network, addr string) (net.Conn, error) {
		dialedNetwork, dialedAddr = network, addr
		return nil, errors.New("dial refused (fake)")
	}
	rt := NewRouter(logger, testBrand(), db, WithNetworkShareSecrets(setter))
	rt.doctorDialContext = fakeDialer
	cookie := loginTestSession(t, rt, db)

	createRec := httptest.NewRecorder()
	createBody := `{"name":"backup-nas","protocol":"cifs","host":"nas.lan","remote_path":"/backups","username":"u","password":"p"}`
	rt.Handler().ServeHTTP(createRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/network-shares", createBody))
	var created networkShareResource
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	testRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(testRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/network-shares/"+created.ID+"/test", ""))
	if testRec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d, body = %s", testRec.Code, http.StatusBadGateway, testRec.Body.String())
	}
	if dialedNetwork != "tcp" || dialedAddr != "nas.lan:445" {
		t.Errorf("dialed %s %s, want tcp nas.lan:445 (cifs default port)", dialedNetwork, dialedAddr)
	}
}
