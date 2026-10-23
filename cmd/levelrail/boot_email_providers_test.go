package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/tokenhash"
)

const (
	loaderRootToken = "loader-root-token"         //nolint:gosec // fake fixture, not a real credential
	loaderSMTPPass  = "api-written-smtp-password" //nolint:gosec // fake fixture, not a real credential
	loaderResendKey = "api-written-resend-key"    //nolint:gosec // fake fixture, not a real credential
)

// providerAPI drives the real provider routes over a real store and secrets manager, then reads back through
// the production loader, so a secret written by the API is proven to be the one the send path resolves.
func providerAPI(t *testing.T) (call func(method, path, body string) *httptest.ResponseRecorder, db *store.DB, mgr *secrets.Manager) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	mgr = secrets.NewManager(db, mk)
	if err := db.SaveAPIToken(ctx, store.APIToken{ID: "tok_loader", Name: "loader", TokenHash: tokenhash.Sum(loaderRootToken), Abilities: []string{api.AbilityRoot}, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	rt := api.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), &brand.Brand{Name: "Test Platform", BinaryName: "testplatform"}, db, api.WithEmailSecrets(mgr))
	h := rt.Handler()
	call = func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+loaderRootToken)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	return call, db, mgr
}

func TestEmailProvidersLoader_ReadsSecretsWrittenThroughTheAPI(t *testing.T) {
	call, db, mgr := providerAPI(t)
	ctx := context.Background()
	load := emailProvidersLoader(db, mgr, nil)

	if rec := call(http.MethodPost, "/api/v1/settings/email/providers", `{"backend":"smtp","smtp_host":"smtp.example.com","smtp_port":587,"smtp_from":"a@example.com","smtp_password":"`+loaderSMTPPass+`"}`); rec.Code != http.StatusCreated {
		t.Fatalf("create primary = %d, body = %s", rec.Code, rec.Body.String())
	}
	rec := call(http.MethodPost, "/api/v1/settings/email/providers", `{"backend":"resend","resend_from":"b@example.com","resend_api_key":"`+loaderResendKey+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create backup = %d, body = %s", rec.Code, rec.Body.String())
	}

	provs, err := load(ctx)
	if err != nil || len(provs) != 2 {
		t.Fatalf("loader = %+v, err = %v, want two providers", provs, err)
	}
	if provs[0].Config.SMTP == nil || provs[0].Config.SMTP.Password != loaderSMTPPass {
		t.Errorf("primary password not read back from the shared namespace: %+v", provs[0].Config.SMTP)
	}
	if provs[1].Config.Resend == nil || provs[1].Config.Resend.APIKey != loaderResendKey {
		t.Errorf("backup key not read back from its own namespace: %+v", provs[1].Config.Resend)
	}
	backupID := provs[1].ID

	if rec := call(http.MethodPost, "/api/v1/settings/email/providers/"+backupID+"/promote", ""); rec.Code != http.StatusOK {
		t.Fatalf("promote = %d, body = %s", rec.Code, rec.Body.String())
	}
	provs, err = load(ctx)
	if err != nil || len(provs) != 2 {
		t.Fatalf("loader after promote = %+v, err = %v", provs, err)
	}
	if provs[0].ID != backupID || provs[0].Config.Resend == nil || provs[0].Config.Resend.APIKey != loaderResendKey {
		t.Errorf("promoted provider lost its credential: %+v", provs[0])
	}
	if provs[1].Config.SMTP == nil || provs[1].Config.SMTP.Password != loaderSMTPPass {
		t.Errorf("demoted provider lost its credential: %+v", provs[1].Config.SMTP)
	}
}

func TestEmailProvidersLoader_OnlyProviderContextPinsOneEvenWhenDisabled(t *testing.T) {
	call, db, mgr := providerAPI(t)
	ctx := context.Background()
	load := emailProvidersLoader(db, mgr, nil)

	call(http.MethodPost, "/api/v1/settings/email/providers", `{"backend":"smtp","smtp_host":"smtp.example.com","smtp_port":587,"smtp_from":"a@example.com","smtp_password":"`+loaderSMTPPass+`"}`)
	call(http.MethodPost, "/api/v1/settings/email/providers", `{"backend":"resend","resend_from":"b@example.com","resend_api_key":"`+loaderResendKey+`","enabled":false}`)

	provs, err := load(ctx)
	if err != nil || len(provs) != 1 {
		t.Fatalf("normal load with a disabled backup = %+v, err = %v, want only the primary", provs, err)
	}
	rows, _ := db.ListEmailProviders(ctx)
	var backupID string
	for _, r := range rows {
		if r.Role == store.EmailRoleBackup {
			backupID = r.ID
		}
	}
	pinned, err := load(email.WithOnlyProvider(ctx, backupID))
	if err != nil || len(pinned) != 1 || pinned[0].ID != backupID || pinned[0].Config.Resend == nil || pinned[0].Config.Resend.APIKey != loaderResendKey {
		t.Errorf("pinned load = %+v, err = %v, want the disabled backup alone", pinned, err)
	}
	if _, err := load(email.WithOnlyProvider(ctx, "prov_missing")); err == nil {
		t.Error("pinning an unknown provider must fail")
	}
}

func TestEmailProvidersLoader_ShimWriteIsWhatTheLoaderSends(t *testing.T) {
	call, db, mgr := providerAPI(t)
	load := emailProvidersLoader(db, mgr, nil)

	rec := call(http.MethodPut, "/api/v1/settings/email", `{"backend":"smtp","from_address":"ops@example.com","smtp_host":"legacy.example.com","smtp_port":465,"smtp_password":"`+loaderSMTPPass+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("shim put = %d, body = %s", rec.Code, rec.Body.String())
	}
	provs, err := load(context.Background())
	if err != nil || len(provs) != 1 || provs[0].Config.SMTP == nil {
		t.Fatalf("loader = %+v, err = %v", provs, err)
	}
	if provs[0].Config.SMTP.Addr != "legacy.example.com:465" || provs[0].Config.SMTP.Password != loaderSMTPPass {
		t.Errorf("shim write not what the loader sends with: %+v", provs[0].Config.SMTP)
	}
}
