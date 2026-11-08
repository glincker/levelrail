package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretprovider"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestSecretProviderResourceNeverSerializesToken(t *testing.T) {
	cfg := secretprovider.Config{Name: "dp", Type: secretprovider.TypeDoppler, Project: "p", Environment: "e", GraceUntil: time.Now().Add(time.Hour)}
	res := toSecretProviderResource(cfg, true)
	for _, v := range []any{res, secretProviderListResponse{Providers: []secretProviderResource{res}}, secretProviderTestResponse{}} {
		typ := reflect.TypeOf(v)
		for i := 0; i < typ.NumField(); i++ {
			tag := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
			if strings.Contains(tag, "token") && tag != "has_token" {
				t.Fatalf("%s.%s serializes %q", typ.Name(), typ.Field(i).Name, tag)
			}
		}
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), `"token"`) {
			t.Fatalf("response carries a token field: %s", raw)
		}
	}
}

func TestIdentityClientIDReturnsOnlyTheID(t *testing.T) {
	id, secret, ok := secretprovider.ParseIdentity(secretprovider.IdentityPrefix + "cid-1:s3cret-value")
	if !ok || id != "cid-1" {
		t.Fatalf("parse: %q %v", id, ok)
	}
	res := secretProviderResource{Name: "inf", Type: secretprovider.TypeInfisical, ClientID: id}
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"client_id":"cid-1"`) || strings.Contains(string(raw), secret) {
		t.Fatalf("resource must carry the ID and never the secret: %s", raw)
	}
}

func TestSecretProviderOnePasswordNeedsNoProjectOrEnvironment(t *testing.T) {
	cfg := secretprovider.Config{Name: "vaults", Type: secretprovider.TypeOnePassword, BaseURL: "https://connect.internal:8080"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("onepassword without project and environment: %v", err)
	}
	cfg.BaseURL = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("onepassword without base_url must fail")
	}
}

// fakeProviderSecrets is an in-memory secret store; the embedded nil SecretSetter is never called by these handlers.
type fakeProviderSecrets struct {
	SecretSetter
	vals map[string]string
	fail map[string]error
}

func (f *fakeProviderSecrets) k(owner, key string) string { return owner + "/" + key }

func (f *fakeProviderSecrets) SetValue(_ context.Context, owner, key, v string) error {
	f.vals[f.k(owner, key)] = v
	return nil
}

func (f *fakeProviderSecrets) Resolve(_ context.Context, owner, key string) (string, error) {
	v, ok := f.vals[f.k(owner, key)]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

func (f *fakeProviderSecrets) Exists(_ context.Context, owner, key string) (bool, error) {
	if err := f.fail["exists"]; err != nil {
		return false, err
	}
	_, ok := f.vals[f.k(owner, key)]
	return ok, nil
}

func (f *fakeProviderSecrets) ListKeys(_ context.Context, owner string) ([]store.SecretKeyInfo, error) {
	var out []store.SecretKeyInfo
	for k := range f.vals {
		if key, ok := strings.CutPrefix(k, owner+"/"); ok {
			out = append(out, store.SecretKeyInfo{Key: key})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func (f *fakeProviderSecrets) DeleteValue(_ context.Context, owner, key string) error {
	delete(f.vals, f.k(owner, key))
	return nil
}

func TestSecretProviderInfisicalClientSecretNeverLeaks(t *testing.T) {
	const secret = "zz-client-secret-9f3a"
	cred := secretprovider.IdentityPrefix + "cid-7:" + secret
	fake := &fakeProviderSecrets{vals: map[string]string{}, fail: map[string]error{}}
	var logs bytes.Buffer
	rt := &Router{secrets: fake, logger: slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))}

	do := func(h http.HandlerFunc, method, name, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/v1/settings/secret-providers/"+name, strings.NewReader(body))
		req.SetPathValue("name", name)
		rec := httptest.NewRecorder()
		h(rec, req)
		return rec
	}
	put := func(token string) string {
		b, _ := json.Marshal(putSecretProviderRequest{Type: secretprovider.TypeInfisical, Project: "p", Environment: "prod", Token: token})
		return string(b)
	}
	assertClean := func(step string, rec *httptest.ResponseRecorder) {
		t.Helper()
		if strings.Contains(rec.Body.String(), secret) {
			t.Fatalf("%s: response carries the client secret: %s", step, rec.Body.String())
		}
	}

	rec := do(rt.handlePutSecretProvider, http.MethodPut, "inf", put(cred))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"client_id":"cid-7"`) {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	assertClean("create", rec)

	rec = do(rt.handleListSecretProviders, http.MethodGet, "", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"has_token":true`) {
		t.Fatalf("read: %d %s", rec.Code, rec.Body.String())
	}
	assertClean("read", rec)

	// Rotating to a new secret keeps the old one only in the store, never in the reply.
	rec = do(rt.handlePutSecretProvider, http.MethodPut, "inf", put(secretprovider.IdentityPrefix+"cid-7:"+secret+"-next"))
	assertClean("update", rec)

	rec = do(rt.handlePutSecretProvider, http.MethodPut, "inf", put(secretprovider.IdentityPrefix+secret))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed credential: %d %s", rec.Code, rec.Body.String())
	}
	assertClean("malformed credential", rec)

	// A token stored for a provider whose token went missing reports a generic reason.
	delete(fake.vals, fake.k(secretprovider.OwnerKey, "TOKEN_INF"))
	rec = do(rt.handleTestSecretProvider, http.MethodPost, "inf", "")
	assertClean("test with unreadable token", rec)

	fake.fail["exists"] = errors.New("backend unavailable")
	rec = do(rt.handleDeleteSecretProvider, http.MethodDelete, "inf", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("delete error path: %d %s", rec.Code, rec.Body.String())
	}
	assertClean("delete error", rec)
	rec = do(rt.handleListSecretProviders, http.MethodGet, "", "")
	assertClean("list error", rec)

	if logs.Len() == 0 {
		t.Fatal("error path logged nothing, the log assertion would be vacuous")
	}
	if strings.Contains(logs.String(), secret) {
		t.Fatalf("log carries the client secret: %s", logs.String())
	}
}
