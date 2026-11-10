package api

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

const bundleCompose = "# Exported from Levelrail app \"shop\". Plain Docker Compose, no Levelrail needed.\nname: \"shop\"\nservices:\n  web:\n    image: \"acme/web:1\"\n"

type bundleEntry struct {
	name string
	body string
	kind byte
}

func makeBundle(t *testing.T, entries ...bundleEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		kind := e.kind
		if kind == 0 {
			kind = tar.TypeReg
		}
		h := &tar.Header{Name: e.name, Mode: 0o644, Typeflag: kind, Size: int64(len(e.body))}
		if kind != tar.TypeReg {
			h.Size, h.Linkname = 0, "/etc/passwd"
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if kind == tar.TypeReg {
			_, _ = tw.Write([]byte(e.body))
		}
	}
	_ = tw.Close()
	return buf.Bytes()
}

func TestReadBundle_HostileArchives(t *testing.T) {
	t.Parallel()
	lim := bundleLimits{maxBytes: 1 << 20, maxEntries: 4, maxEntryBytes: 256}
	compose := bundleEntry{name: bundleComposeName, body: bundleCompose}
	cases := []struct {
		name    string
		entries []bundleEntry
	}{
		{"traversal", []bundleEntry{compose, {name: "../evil", body: "x"}}},
		{"absolute", []bundleEntry{compose, {name: "/etc/cron.d/x", body: "x"}}},
		{"nested", []bundleEntry{compose, {name: "a/README.md", body: "x"}}},
		{"symlink", []bundleEntry{compose, {name: "README.md", kind: tar.TypeSymlink}}},
		{"hardlink", []bundleEntry{compose, {name: "README.md", kind: tar.TypeLink}}},
		{"device", []bundleEntry{compose, {name: "README.md", kind: tar.TypeChar}}},
		{"duplicate", []bundleEntry{compose, compose}},
		{"case collision", []bundleEntry{compose, {name: "README.md", body: "a"}, {name: "readme.md", body: "b"}}},
		{"oversize entry", []bundleEntry{compose, {name: "README.md", body: strings.Repeat("x", 300)}}},
		{"too many entries", []bundleEntry{compose, {name: "README.md"}, {name: "NOTES.md"}, {name: "volumes.json"}, {name: bundleEnvName}}},
		{"clear secrets", []bundleEntry{compose, {name: ".env", body: "TOKEN=abc"}}},
		{"unknown entry", []bundleEntry{compose, {name: "run.sh", body: "x"}}},
		{"no compose", []bundleEntry{{name: "README.md", body: "x"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := readBundle(makeBundle(t, tc.entries...), lim)
			if !errors.Is(err, errBundleInvalid) {
				t.Fatalf("err = %v, want errBundleInvalid", err)
			}
		})
	}
}

func TestCheckBundleVersion(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		manifest string
		compose  string
		ok       bool
	}{
		{"marker only", "", bundleCompose, true},
		{"no marker", "", "services: {}\n", false},
		{"v1 manifest", `{"format":"levelrail-app-bundle","version":"1.2"}`, "services: {}\n", true},
		{"unknown major", `{"format":"levelrail-app-bundle","version":"2.0"}`, bundleCompose, false},
		{"unknown field", `{"format":"levelrail-app-bundle","version":"1","extra":1}`, bundleCompose, false},
		{"wrong format", `{"format":"other","version":"1"}`, bundleCompose, false},
	}
	for _, tc := range cases {
		files := map[string][]byte{bundleComposeName: []byte(tc.compose)}
		if tc.manifest != "" {
			files[bundleManifestName] = []byte(tc.manifest)
		}
		if _, err := checkBundleVersion(files); (err == nil) != tc.ok {
			t.Errorf("%s: err = %v, want ok %v", tc.name, err, tc.ok)
		}
	}
}

func TestImportAppBundle_RoundTripDryRunAndClash(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{
		Name: "web", Image: "acme/web:1", Port: 3000,
		Env: map[string]string{"MODE": "prod", "API_TOKEN": exportCanary},
	}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/export?format=compose", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("export status = %d", rec.Code)
	}
	bundle := rec.Body.String()
	post := func(query string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/import-bundle?"+query, bundle))
		return rec
	}

	if rec := post(""); rec.Code != http.StatusConflict {
		t.Fatalf("clash status = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := post("replace=true&confirm=nope"); rec.Code != http.StatusForbidden {
		t.Fatalf("replace without confirm status = %d", rec.Code)
	}

	rec = post("name=web2&dry_run=true")
	if rec.Code != http.StatusOK {
		t.Fatalf("dry run status = %d: %s", rec.Code, rec.Body.String())
	}
	var plan appBundleImportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); err != nil || !plan.DryRun || len(plan.Services) != 1 || len(plan.SHA256) != 64 {
		t.Fatalf("plan = %+v, err = %v", plan, err)
	}
	if len(plan.SecretsToSet) != 1 || plan.SecretsToSet[0] != "API_TOKEN" {
		t.Fatalf("secrets to set = %v", plan.SecretsToSet)
	}
	if _, err := db.GetDesiredService(ctx, plan.Services[0].Name); !errors.Is(err, store.ErrServiceNotFound) {
		t.Fatalf("dry run wrote a service: %v", err)
	}

	rec = post("name=web2")
	if rec.Code != http.StatusCreated {
		t.Fatalf("import status = %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), exportCanary) {
		t.Fatal("response echoes a secret value")
	}
	got, err := db.GetDesiredService(ctx, plan.Services[0].Name)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Suspended || got.Image != "acme/web:1" || got.Env["MODE"] != "prod" || got.Env["API_TOKEN"] == exportCanary {
		t.Fatalf("imported service = suspended %v image %q env %v", got.Suspended, got.Image, got.Env)
	}
	entries, err := db.ListAuditEntries(ctx, 50, nil, store.AuditEntryFilter{Action: AuditActionAppImportBundle})
	if err != nil || len(entries) != 1 || !strings.Contains(entries[0].Detail, plan.SHA256) || strings.Contains(entries[0].Detail, exportCanary) {
		t.Fatalf("audit = %+v, err = %v", entries, err)
	}
	if rec := post("name=web2"); rec.Code != http.StatusConflict {
		t.Fatalf("second import status = %d", rec.Code)
	}
}

func bundleBody(t *testing.T) string {
	t.Helper()
	return string(makeBundle(t, bundleEntry{name: bundleComposeName, body: bundleCompose}))
}

func TestImportAppBundle_IAMDenyIsNotFound(t *testing.T) {
	t.Parallel()
	e := newBuilderEnv(t)
	tok := seedMatrixToken(t, e.db, "importer", []string{AbilityRead, AbilityWrite, AbilityDeploy})
	e.attach(t, e.seedPolicy(t, "no-prod", docOf("Deny", `"*"`, `"app:prod"`)), store.PrincipalTypeToken, "importer")
	for _, q := range []string{"name=prod&dry_run=true", "name=prod"} {
		rec := httptest.NewRecorder()
		e.rt.Handler().ServeHTTP(rec, bearerRequest(http.MethodPost, "/api/v1/apps/import-bundle?"+q, bundleBody(t), tok))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d, want 404: %s", q, rec.Code, rec.Body.String())
		}
	}
	if _, err := e.db.GetDesiredService(context.Background(), "prod-web"); !errors.Is(err, store.ErrServiceNotFound) {
		t.Fatalf("denied import wrote a service: %v", err)
	}
}

func TestImportAppBundle_ReplaceHonorsStackLock(t *testing.T) {
	rt, db, cookie, _ := stackLockFixture(t, "lock")
	path := "/api/v1/apps/import-bundle?name=web&replace=true&confirm=web&dry_run=true"
	if code, out := stackLockCall(t, rt, authedRequest(t, cookie, http.MethodPost, path, bundleBody(t))); code != http.StatusConflict || out.Code != "stack_locked" {
		t.Fatalf("locked replace = %d %+v, want 409 stack_locked", code, out)
	}
	req := authedRequest(t, cookie, http.MethodPost, path, bundleBody(t))
	req.Header.Set("X-Levelrail-Override-Stack-Lock", "shop-prod")
	if code, out := stackLockCall(t, rt, req); out.Code == "stack_locked" {
		t.Fatalf("root override refused: %d", code)
	}
	entries, err := db.ListAuditEntries(t.Context(), 50, nil, store.AuditEntryFilter{Action: "stack.lock_override"})
	if err != nil || len(entries) != 1 {
		t.Fatalf("override audit = %d, %v", len(entries), err)
	}
}

// suspendOnlyCompose fails the test when an import saves a running row.
type suspendOnlyCompose struct {
	AppComposeStore
	t *testing.T
}

func (s suspendOnlyCompose) SaveDesiredService(context.Context, store.DesiredService) error {
	s.t.Error("import saved a service without the suspended flag")
	return errors.New("unexpected unsuspended save")
}

func TestImportAppBundle_SavesSuspendedAtomically(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rt.appCompose = suspendOnlyCompose{AppComposeStore: rt.appCompose, t: t}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/import-bundle?name=shop", bundleBody(t)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	got, err := db.GetDesiredService(context.Background(), "shop-web")
	if err != nil || !got.Suspended {
		t.Fatalf("service = %+v, err = %v", got, err)
	}
}

func TestNormalizeBundlePorts_OnlyPortsBlocks(t *testing.T) {
	t.Parallel()
	in := "services:\n  web:\n    command:\n      - \"127.0.0.1:80:80\"\n    ports:\n      - \"127.0.0.1:3000:3000\"\n      - \"127.0.0.1:1:2\"\n    environment:\n      - \"127.0.0.1:9:9\"\n"
	want := "services:\n  web:\n    command:\n      - \"127.0.0.1:80:80\"\n    ports:\n      - \"3000\"\n      - \"127.0.0.1:1:2\"\n    environment:\n      - \"127.0.0.1:9:9\"\n"
	if got := string(normalizeBundlePorts([]byte(in))); got != want {
		t.Fatalf("got:\n%s", got)
	}
}
