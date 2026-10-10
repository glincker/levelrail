package appimport

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/datamigrate"
	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	pgUUID  = "k0pgx9uuid"
	pgNew   = "pg-main.mesh"
	dbURL   = "postgres://app:hunter2@k0pgx9uuid:5432/app"   //nolint:gosec // fixture value
	rewrite = "postgres://app:hunter2@pg-main.mesh:5432/app" //nolint:gosec // fixture value
)

func discovery() *platformimport.Discovery {
	return &platformimport.Discovery{
		Platform: platformimport.Coolify,
		Databases: []platformimport.Database{
			{SourceID: "d1", Name: "main-db", Engine: "postgres", Version: "16", InternalHost: pgUUID},
			{SourceID: "d2", Name: "cold-db", Engine: "postgres", Version: "15", InternalHost: "cold-uuid"},
		},
		Apps: []platformimport.App{
			{SourceID: "a-img", Name: "searxng", Project: "P", Environment: "prod", Kind: platformimport.SourceImage, Image: "searxng/searxng:1", Port: 8080,
				Env: []platformimport.Env{{Key: "MODE", Value: "x"}}},
			{SourceID: "a-git", Name: "web", Project: "P", Environment: "prod", Kind: platformimport.SourceGit, GitURL: "https://github.com/a/web", GitBranch: "main",
				BuildMethod: "dockerfile", BuildPack: "dockerfile", Port: 3000, Domains: []string{"web.example.com"},
				Env: []platformimport.Env{{Key: "DATABASE_URL", Value: dbURL, Secret: true}}},
			{SourceID: "a-cold", Name: "report", Project: "P", Environment: "prod", Kind: platformimport.SourceImage, Image: "r:1", Port: 80,
				Env: []platformimport.Env{{Key: "COLD", Value: "postgres://u:p@cold-uuid:5432/x"}}}, //nolint:gosec // fixture value
			{SourceID: "a-sec", Name: "api", Project: "P", Environment: "staging", Kind: platformimport.SourceImage, Image: "api:1", Port: 80,
				Env: []platformimport.Env{{Key: "API_KEY", Secret: true}}, Volumes: []platformimport.Volume{{Name: "data", ContainerPath: "/data"}}},
			{SourceID: "a-nix", Name: "nix", Project: "P", Environment: "prod", Kind: platformimport.SourceGit, GitURL: "u", BuildMethod: "railpack", BuildPack: "nixpacks", Port: 1,
				Notes: []platformimport.Note{{Reason: "source used Nixpacks; imported as Railpack auto-detect, which may build differently", Manual: "compare"}}},
			{SourceID: "a-shared", Name: "shared", Project: "P", Environment: "prod", Kind: platformimport.SourceImage, Image: "s:1", Port: 1,
				Env: []platformimport.Env{{Key: "K", Value: "{{project.TOKEN}}"}}},
			{SourceID: "a-priv", Name: "priv", Project: "P", Environment: "prod", Kind: platformimport.SourceGit, GitURL: "u", BuildMethod: "dockerfile", Port: 1, PrivateRepo: true},
		},
		Unsupported: []platformimport.Unsupported{
			{Kind: "app", SourceID: "u-compose", Name: "stack", Reason: "application uses the Docker Compose build pack", Manual: "deploy its compose file", Project: "P", Environment: "prod"},
			{Kind: "service", SourceID: "u-svc", Name: "plausible", Reason: "one-click service", Manual: "export compose", Project: "P", Environment: "prod"},
		},
	}
}

func testContext() Context {
	return Context{Targets: map[string]string{"main-db": "main-db"}, TargetHost: func(string) string { return pgNew }}
}

func TestVerdictRules(t *testing.T) {
	inv := BuildInventory(discovery(), testContext())
	got := map[string]Entry{}
	for _, e := range inv.Entries() {
		got[e.SourceID] = e
	}
	tests := []struct {
		id      string
		verdict string
		reason  string
	}{
		{"a-img", VerdictReady, ""},
		{"a-git", VerdictNotes, "already moved to main-db"},
		{"a-cold", VerdictAttention, "not moved here yet"},
		{"a-sec", VerdictAttention, "no readable value"},
		{"a-nix", VerdictNotes, "Nixpacks"},
		{"a-shared", VerdictAttention, "shared variables"},
		{"a-priv", VerdictAttention, "private"},
		{"u-compose", VerdictUnsupported, "Compose"},
		{"u-svc", VerdictUnsupported, "one-click"},
	}
	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			e, ok := got[tc.id]
			if !ok {
				t.Fatalf("missing entry")
			}
			if e.Verdict != tc.verdict {
				t.Errorf("verdict = %s, want %s (%+v)", e.Verdict, tc.verdict, e.Findings)
			}
			found := tc.reason == ""
			for _, f := range e.Findings {
				if strings.Contains(strings.ToLower(f.Reason), strings.ToLower(tc.reason)) {
					found = true
				}
			}
			if !found {
				t.Errorf("no finding contains %q: %+v", tc.reason, e.Findings)
			}
		})
	}
	if len(inv.Groups) != 2 {
		t.Errorf("groups = %d, want prod and staging", len(inv.Groups))
	}
	if inv.Counts[VerdictUnsupported] != 2 {
		t.Errorf("counts = %v", inv.Counts)
	}
	if e := got["a-git"]; len(e.Databases) != 1 || e.Databases[0].TargetHost != pgNew {
		t.Errorf("db dependency not found: %+v", e.Databases)
	}
}

func TestRewriteValue(t *testing.T) {
	maps := []Mapping{{From: "k0pgx9uuid", To: "pg-main.mesh"}}
	tests := []struct {
		name, in, out string
		n             int
	}{
		{"url host", dbURL, rewrite, 1},
		{"bare host", "k0pgx9uuid", "pg-main.mesh", 1},
		{"host and port", "k0pgx9uuid:5432", "pg-main.mesh:5432", 1},
		{"prefix of longer name", "k0pgx9uuid-replica:5432", "k0pgx9uuid-replica:5432", 0},
		{"suffix of longer name", "mk0pgx9uuid", "mk0pgx9uuid", 0},
		{"subdomain left alone", "k0pgx9uuid.example.com", "k0pgx9uuid.example.com", 0},
		{"case insensitive", "PG://K0PGX9UUID/db", "PG://pg-main.mesh/db", 1},
		{"twice", "k0pgx9uuid,k0pgx9uuid", "pg-main.mesh,pg-main.mesh", 2},
		{"password equal to host untouched user part only", "postgres://u:x@k0pgx9uuid/d", "postgres://u:x@pg-main.mesh/d", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, n := RewriteValue(tc.in, maps)
			if out != tc.out || n != tc.n {
				t.Errorf("got %q (%d), want %q (%d)", out, n, tc.out, tc.n)
			}
		})
	}
}

func TestRewriteEnvDiffNeverShowsSecrets(t *testing.T) {
	env := []platformimport.Env{
		{Key: "DATABASE_URL", Value: dbURL, Secret: true},
		{Key: "DB_HOST", Value: pgUUID},
		{Key: "OPAQUE_SECRET", Value: "tok:" + pgUUID + ":zz", Secret: true},
		{Key: "UNRELATED", Value: "x"},
	}
	out, ch := RewriteEnv("web", env, []Mapping{{From: pgUUID, To: pgNew}})
	if env[0].Value != dbURL {
		t.Fatal("input was modified")
	}
	if out[0].Value != rewrite {
		t.Errorf("value = %q", out[0].Value)
	}
	blob, _ := json.Marshal(ch)
	for _, leak := range []string{"hunter2", "tok:"} {
		if strings.Contains(string(blob), leak) {
			t.Errorf("diff leaks %q: %s", leak, blob)
		}
	}
	if len(ch) != 3 {
		t.Errorf("changes = %d, want 3", len(ch))
	}
	if !strings.Contains(string(blob), "****") {
		t.Errorf("url password not masked: %s", blob)
	}
}

func TestValidateMapping(t *testing.T) {
	tests := []struct {
		m  Mapping
		ok bool
	}{
		{Mapping{"a", "b"}, true},
		{Mapping{"", "b"}, false},
		{Mapping{"a b", "c"}, false},
		{Mapping{"a", "a"}, false},
		{Mapping{"a", "x;rm"}, false},
	}
	for _, tc := range tests {
		if err := ValidateMapping(tc.m); (err == nil) != tc.ok {
			t.Errorf("%+v: err=%v", tc.m, err)
		}
	}
}

func preflightInput(maps []Mapping) PreflightInput {
	d := discovery()
	env := map[string][]platformimport.Env{}
	for _, a := range d.Apps {
		env[a.SourceID] = a.Env
	}
	return PreflightInput{
		Inventory: BuildInventory(d, testContext()), Env: env, Mappings: maps, FreeBytes: -1,
		Names:        map[string]string{"a-img": "searxng-2"},
		DomainOwners: map[string]string{"web.example.com": "legacy-web"},
		Selected:     map[string]bool{"a-img": true, "a-git": true, "a-sec": true},
	}
}

func TestPreflightCollisionsAndHostnames(t *testing.T) {
	res := Preflight(preflightInput(nil))
	find := func(app, id, status string) bool {
		for _, c := range res.Checks {
			if c.App == app && c.ID == id && c.Status == status {
				return true
			}
		}
		return false
	}
	tests := []struct {
		name, app, id, status string
	}{
		{"name taken", "searxng", CheckName, datamigrate.CheckWarn},
		{"domain conflict", "web", CheckDomains, datamigrate.CheckFail},
		{"unmapped db host", "web", CheckDBHosts, datamigrate.CheckFail},
		{"secrets listed", "web", CheckSecrets, datamigrate.CheckPass},
	}
	for _, tc := range tests {
		if !find(tc.app, tc.id, tc.status) {
			t.Errorf("%s: missing %s/%s/%s in %+v", tc.name, tc.app, tc.id, tc.status, res.Checks)
		}
	}
	if res.CanStage {
		t.Error("a domain conflict must block staging")
	}

	in := preflightInput([]Mapping{{From: pgUUID, To: pgNew}})
	in.DomainOwners = nil
	res = Preflight(in)
	if !res.CanStage || len(res.Diff) != 1 || res.Diff[0].App != "web" {
		t.Errorf("mapped preflight: can=%v diff=%+v checks=%+v", res.CanStage, res.Diff, res.Checks)
	}
	if res.Failed != 0 {
		t.Errorf("failed = %d: %+v", res.Failed, res.Checks)
	}
}

func TestPreflightDuplicateDomainAcrossApps(t *testing.T) {
	in := preflightInput(nil)
	in.DomainOwners = nil
	in.Selected = map[string]bool{"a-git": true, "a-dup": true}
	d := discovery()
	d.Apps = append(d.Apps, platformimport.App{SourceID: "a-dup", Name: "web2", Kind: platformimport.SourceImage, Image: "i", Port: 1, Domains: []string{"web.example.com"}})
	in.Inventory = BuildInventory(d, testContext())
	res := Preflight(in)
	found := false
	for _, c := range res.Checks {
		if c.ID == CheckDomains && c.Status == datamigrate.CheckFail && strings.Contains(c.Detail, "also used") {
			found = true
		}
	}
	if !found {
		t.Errorf("duplicate domain not caught: %+v", res.Checks)
	}
}

func TestPreflightDisk(t *testing.T) {
	d := &platformimport.Discovery{Apps: []platformimport.App{
		{SourceID: "v", Name: "v", Kind: platformimport.SourceImage, Image: "i", Port: 1, Volumes: []platformimport.Volume{{Name: "a", ContainerPath: "/a", SizeBytes: 10 << 30}, {Name: "b", ContainerPath: "/b"}}},
	}}
	tests := []struct {
		name   string
		free   int64
		status string
	}{
		{"not enough", 1 << 30, datamigrate.CheckFail},
		{"unknown sizes warn", 100 << 30, datamigrate.CheckWarn},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := Preflight(PreflightInput{Inventory: BuildInventory(d, Context{}), FreeBytes: tc.free})
			for _, c := range res.Checks {
				if c.ID == CheckDisk && c.Status == tc.status {
					return
				}
			}
			t.Errorf("no disk check with %s: %+v", tc.status, res.Checks)
		})
	}
}

func TestReceiptHoldsNoSecretAndListsRemaining(t *testing.T) {
	inv := BuildInventory(discovery(), testContext())
	var sec Entry
	for _, e := range inv.Entries() {
		if e.SourceID == "a-sec" {
			sec = e
		}
	}
	item := store.AppImportItem{SourceID: "a-sec", SourceName: "api", TargetName: "api", Selected: true, State: store.AppImportStaged,
		Volumes: []store.AppImportVolume{{Name: "data", ContainerPath: "/data"}}, Domains: []string{"api.example.com"}}
	r := BuildReceipt(store.AppImportSession{ID: "s1", Platform: "coolify", SourceURL: "https://c.example.com",
		Mappings: []store.AppImportMapping{{From: pgUUID, To: pgNew}}}, []ReceiptInput{{Item: item, Entry: sec, Rewritten: []AppliedChange{{Key: "DATABASE_URL", Count: 1}}}}, time.Unix(0, 0))
	blob, _ := json.Marshal(r)
	if strings.Contains(string(blob), "hunter2") {
		t.Fatal("receipt leaks a value")
	}
	if r.Remaining < 3 {
		t.Errorf("remaining = %d (%v)", r.Remaining, r.Apps[0].Remaining)
	}
	joined := strings.Join(r.Apps[0].Remaining, "|")
	for _, want := range []string{"build and verify", "copy volume data", "switch DNS for api.example.com"} {
		if !strings.Contains(joined, want) {
			t.Errorf("remaining lacks %q: %s", want, joined)
		}
	}
}
