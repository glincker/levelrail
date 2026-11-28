package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/store"
)

const hostedPass = "Hosted-pw-7" //nolint:gosec // fake fixture

const (
	hostedProbeOK   = "app|appdb|PostgreSQL 16.2|appdb\nLR_EXIT:0\n"
	hostedProbeFail = "psql: error: connection to server failed: Connection refused\nLR_EXIT:2\n"
)

func hostedURL() string {
	return `{"name":"neon-main","url":"postgres://app:` + hostedPass + `@ep-cool-1.eu-central-1.aws.neon.tech/appdb?sslmode=require"}`
}

func TestHostedDatabase_SaveFlow(t *testing.T) {
	rt, db, fake, sec, logs := newExternalTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	// A failing probe blocks the save and stores nothing.
	fake.execOut = hostedProbeFail
	rec := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/external-databases", hostedURL())
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"probe_failed"`) || !strings.Contains(rec.Body.String(), `"unreachable"`) {
		t.Fatalf("failing probe = %d %s", rec.Code, rec.Body.String())
	}
	if _, err := db.GetExternalDatabase(t.Context(), "neon-main"); err == nil {
		t.Fatal("record saved despite a failed probe")
	}
	if len(sec.vals) != 0 {
		t.Fatalf("password stored despite a failed probe: %v", sec.vals)
	}

	// save_anyway overrides the block.
	rec = viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/external-databases", strings.Replace(hostedURL(), `"name"`, `"save_anyway":true,"name"`, 1))
	if rec.Code != http.StatusCreated {
		t.Fatalf("save anyway = %d %s", rec.Code, rec.Body.String())
	}
	if err := db.DeleteExternalDatabase(t.Context(), "neon-main"); err != nil {
		t.Fatal(err)
	}

	// A reachable database saves with fields taken from the URL.
	fake.execOut = hostedProbeOK
	rec = viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/external-databases", hostedURL())
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	saved, err := db.GetExternalDatabase(t.Context(), "neon-main")
	if err != nil {
		t.Fatal(err)
	}
	if saved.Provider != "neon" || saved.Host != "ep-cool-1.eu-central-1.aws.neon.tech" || saved.TLSMode != "verify-full" || saved.Username != "app" || saved.DatabaseName != "appdb" {
		t.Fatalf("saved = %+v", saved)
	}
	if saved.HealthStatus != "reachable" {
		t.Errorf("health = %q, want the preflight result", saved.HealthStatus)
	}
	if got := sec.vals["external-database/neon-main/password"]; got != hostedPass {
		t.Errorf("stored secret = %q", got)
	}
	all := rec.Body.String()
	for _, p := range []string{"/api/v1/external-databases", "/api/v1/databases"} {
		all += viewerDo(t, rt, cookie, http.MethodGet, p, "").Body.String()
	}
	all += viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/external-databases/parse", `{"url":"postgres://u:`+hostedPass+`@db.x.supabase.co/p"}`).Body.String()
	if strings.Contains(all, hostedPass) || strings.Contains(logs.String(), hostedPass) {
		t.Fatal("password leaked through a response or a log")
	}
	for _, c := range fake.execCmds {
		if strings.Contains(strings.Join(c, " "), hostedPass) {
			t.Fatalf("password in argv: %v", c)
		}
	}
	last := fake.created[len(fake.created)-1]
	if last.Env["PGSSLMODE"] != "verify-full" || last.Env["PGSSLROOTCERT"] != "/etc/ssl/certs/ca-certificates.crt" {
		t.Errorf("helper TLS env = %v", last.Env)
	}
}

func TestHostedDatabase_ParseAndProviders(t *testing.T) {
	rt, db, _, _, _ := newExternalTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/external-databases/parse", `{"url":"mongodb+srv://u:pw@cluster0.ab1cd.mongodb.net/shop"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"provider":"mongodb-atlas"`) || !strings.Contains(rec.Body.String(), `"has_password":true`) {
		t.Fatalf("parse = %d %s", rec.Code, rec.Body.String())
	}
	rec = viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/external-databases/parse", `{"url":"http://u:`+hostedPass+`@h/x"}`)
	if rec.Code != http.StatusBadRequest || strings.Contains(rec.Body.String(), hostedPass) {
		t.Fatalf("bad scheme = %d %s", rec.Code, rec.Body.String())
	}
	rec = viewerDo(t, rt, cookie, http.MethodGet, "/api/v1/external-databases/providers", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"id":"neon"`) || !strings.Contains(rec.Body.String(), `"id":"generic"`) {
		t.Fatalf("providers = %d %s", rec.Code, rec.Body.String())
	}
}

func TestHostedDatabase_AddressPolicyAndAuthz(t *testing.T) {
	rt, db, fake, _, _ := newExternalTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	fake.execOut = hostedProbeOK
	// The address policy still applies to a host that arrives inside a URL.
	for _, host := range []string{"169.254.169.254", "127.0.0.1", "localhost"} {
		body := `{"name":"x","url":"postgres://u:pw@` + host + `/d"}`
		rec := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/external-databases", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("host %s = %d, want 400", host, rec.Code)
		}
	}
	if len(fake.created) != 0 {
		t.Fatalf("a helper was started for a blocked host: %d", len(fake.created))
	}

	// A read scoped token cannot save, test, parse or list presets.
	const plaintext = "hosted-read-token" //nolint:gosec // fake fixture
	if err := db.SaveAPIToken(t.Context(), store.APIToken{
		ID: "tok_hosted_read", Name: "read", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/external-databases"},
		{http.MethodPost, "/api/v1/external-databases/test"},
		{http.MethodPost, "/api/v1/external-databases/parse"},
		{http.MethodGet, "/api/v1/external-databases/providers"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(hostedURL()))
		req.Header.Set("Authorization", "Bearer "+plaintext)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s with a read token = %d, want 403", tc.method, tc.path, rec.Code)
		}
	}
	if len(fake.created) != 0 {
		t.Fatalf("a helper was started for a denied request: %d", len(fake.created))
	}
}

func TestHostedDatabase_NoLeaksForOtherEngines(t *testing.T) {
	rt, db, fake, _, logs := newExternalTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	cases := map[string]struct{ url, out string }{
		"my-mysql": {"mysql://app:" + hostedPass + "@aws.connect.psdb.cloud/shop", "app@%\tshop\t8.0.36\tshop\nLR_EXIT:0\n"},
		"my-redis": {"rediss://default:" + hostedPass + "@eu1-calm-1.upstash.io:6379", "PONG\nLR_EXIT:0\n"},
		"my-mongo": {"mongodb+srv://app:" + hostedPass + "@cluster0.ab1cd.mongodb.net/shop", `{"ok":1,"version":"7.0"}` + "\nLR_EXIT:0\n"},
	}
	all := ""
	for name, tc := range cases {
		fake.execOut = tc.out
		rec := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/external-databases", `{"name":"`+name+`","url":"`+tc.url+`"}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("%s create = %d %s", name, rec.Code, rec.Body.String())
		}
		all += rec.Body.String()
		all += viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/external-databases/test", `{"url":"`+tc.url+`"}`).Body.String()
		all += viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/external-databases/"+name+"/probe", "").Body.String()
	}
	all += viewerDo(t, rt, cookie, http.MethodGet, "/api/v1/databases", "").Body.String()
	if strings.Contains(all, hostedPass) || strings.Contains(logs.String(), hostedPass) {
		t.Fatal("password leaked through a response or a log")
	}
	for _, c := range fake.execCmds {
		if strings.Contains(strings.Join(c, " "), hostedPass) {
			t.Fatalf("password in argv: %v", c)
		}
	}
	if entries, err := db.ListAuditEntries(t.Context(), 200, nil, store.AuditEntryFilter{}); err == nil {
		for _, e := range entries {
			if strings.Contains(e.Path, hostedPass) {
				t.Fatalf("password in audit path %q", e.Path)
			}
		}
	}
	srv := false
	for _, spec := range fake.created {
		if spec.Env["EXT_SRV"] == "1" {
			srv = true
		}
	}
	if !srv {
		t.Error("the Atlas helper did not use the seed record form")
	}
}

func TestHostedDatabase_UpdateAdoptEngineMismatchAndProbeLimit(t *testing.T) {
	rt, db, fake, _, _ := newExternalTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	fake.execOut = hostedProbeOK
	rec := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/external-databases", hostedURL())
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	rec = viewerDo(t, rt, cookie, http.MethodPut, "/api/v1/external-databases/neon-main", `{"url":"mysql://u:pw@db.example.com/x"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("update with a mysql url = %d, want 400", rec.Code)
	}
	rec = viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/external-databases/adopt", `{"container":"coolify-pg","url":"mysql://u:pw@db.example.com/x"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("adopt with a mysql url = %d, want 400", rec.Code)
	}

	req := authedRequest(t, cookie, http.MethodPost, "/api/v1/external-databases/test", "")
	var releases []func()
	for i := 0; i < 2; i++ {
		rel, ok := rt.acquireExternalProbe(req)
		if !ok {
			t.Fatalf("slot %d refused below the limit", i)
		}
		releases = append(releases, rel)
	}
	rec = viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/external-databases/test", hostedURL())
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("over the limit = %d, want 429", rec.Code)
	}
	for _, rel := range releases {
		rel()
	}
	rec = viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/external-databases/test", hostedURL())
	if rec.Code != http.StatusOK {
		t.Errorf("after release = %d %s", rec.Code, rec.Body.String())
	}
}
