package api

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// oracleListVisibleApps is the pre-change scoped path: load every match, gate in memory, page the slice.
func oracleListVisibleApps(ctx context.Context, fs appFilterStore, f store.AppListFilter, canRead func(string) bool) ([]store.DesiredService, int, error) {
	dbFilter := f
	dbFilter.Limit, dbFilter.Offset = 0, 0
	svcs, _, err := fs.ListDesiredServicesFiltered(ctx, dbFilter)
	if err != nil {
		return nil, 0, err
	}
	visible := svcs[:0:0]
	for _, s := range svcs {
		if canRead(s.Name) {
			visible = append(visible, s)
		}
	}
	return pageSlice(visible, f.Limit, f.Offset), len(visible), nil
}

type diffFixture struct {
	db       *store.DB
	teams    []string
	projects []string
	envs     []store.Environment
	apps     []string
	tags     []string
}

func seedDiffFixture(ctx context.Context, t *testing.T, rng *rand.Rand, db *store.DB) diffFixture {
	t.Helper()
	fx := diffFixture{db: db, teams: []string{store.DefaultTeamID}, tags: []string{"red", "blue", "green"}}
	exec := func(q string, args ...any) {
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	for i := 0; i < 1+rng.Intn(3); i++ {
		id := fmt.Sprintf("org-%d", i)
		if err := db.SaveOrganization(ctx, store.Organization{ID: id, Name: id, CreatedAt: "2026-01-01T00:00:00Z"}); err != nil {
			t.Fatal(err)
		}
		fx.teams = append(fx.teams, id)
	}
	for i := 0; i < 1+rng.Intn(4); i++ {
		id := fmt.Sprintf("proj-%d", i)
		if err := db.SaveProject(ctx, store.Project{ID: id, Name: id, CreatedAt: "2026-01-01T00:00:00Z"}); err != nil {
			t.Fatal(err)
		}
		if team := fx.teams[rng.Intn(len(fx.teams))]; team != store.DefaultTeamID || rng.Intn(2) == 0 {
			exec(`UPDATE projects SET org_id = ? WHERE id = ?`, team, id)
		}
		fx.projects = append(fx.projects, id)
	}
	envNames := []string{"production", "staging", "preview"}
	for i := 0; i < 1+rng.Intn(4); i++ {
		e := store.Environment{ID: fmt.Sprintf("env-%d", i), ProjectID: fx.projects[rng.Intn(len(fx.projects))], Name: envNames[rng.Intn(len(envNames))], Protected: rng.Intn(3) == 0}
		if err := db.SaveEnvironment(ctx, e); err != nil {
			t.Fatal(err)
		}
		fx.envs = append(fx.envs, e)
	}
	for _, tag := range fx.tags {
		if err := db.SaveTag(ctx, store.Tag{ID: "tag-" + tag, Name: tag}); err != nil {
			t.Fatal(err)
		}
	}
	images := []string{"nginx:1", "web/api:2", "redis:7"}
	n := rng.Intn(45)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("app-%02d", rng.Intn(90))
		if rng.Intn(10) == 0 {
			name += store.CanaryServiceSuffix
		}
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: name, Image: images[rng.Intn(len(images))], Port: 80}); err != nil {
			t.Fatal(err)
		}
		if rng.Intn(4) != 0 {
			if err := db.UpdateServiceProject(ctx, name, fx.projects[rng.Intn(len(fx.projects))]); err != nil {
				t.Fatal(err)
			}
		}
		if rng.Intn(3) != 0 {
			if err := db.SetServiceEnvironment(ctx, name, fx.envs[rng.Intn(len(fx.envs))].ID); err != nil {
				t.Fatal(err)
			}
		}
		for _, tag := range fx.tags {
			if rng.Intn(3) == 0 {
				if err := db.AttachAppTag(ctx, "tag-"+tag, name); err != nil {
					t.Fatal(err)
				}
			}
		}
		if rng.Intn(6) == 0 {
			if err := db.UpdateServiceSuspended(ctx, name, true); err != nil {
				t.Fatal(err)
			}
		}
		fx.apps = append(fx.apps, name)
	}
	for _, name := range fx.apps {
		if rng.Intn(8) == 0 {
			_ = db.DeleteDesiredService(ctx, name)
		}
	}
	return fx
}

type diffCaller struct {
	name      string
	abilities []string
	policies  []store.Policy
	scope     *resourceScope
}

func diffPolicy(effect string, resources ...string) store.Policy {
	res := ""
	for i, r := range resources {
		if i > 0 {
			res += ","
		}
		res += strconv.Quote(r)
	}
	return store.Policy{ID: "p-" + effect, Document: `{"Statement":[{"Effect":"` + effect + `","Action":["read"],"Resource":[` + res + `]}]}`}
}

// diffCallers builds real resourceScope values so canRead is the production predicate for each caller class.
func diffCallers(ctx context.Context, t *testing.T, rng *rand.Rand, fx diffFixture) []diffCaller {
	t.Helper()
	owners, err := fx.db.OwnersOfApps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	envs, err := fx.db.EnvironmentsOfApps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	scope := func(vis store.Visibility, teams *teamScope) *resourceScope {
		return &resourceScope{vis: vis, appEnv: envs, teams: teams, appOwner: owners}
	}
	team := func() string { return fx.teams[rng.Intn(len(fx.teams))] }
	app := func() string {
		if len(fx.apps) == 0 {
			return "none"
		}
		return fx.apps[rng.Intn(len(fx.apps))]
	}
	restricted := store.Visibility{Restricted: true, EnvironmentIDs: map[string]bool{}}
	for _, e := range fx.envs {
		if rng.Intn(2) == 0 {
			restricted.EnvironmentIDs[e.ID] = true
		}
	}
	env := fx.envs[rng.Intn(len(fx.envs))]
	roles := []string{store.TeamRoleOwner, store.TeamRoleAdmin, store.TeamRoleDeveloper, store.TeamRoleViewer}
	return []diffCaller{
		{name: "admin", abilities: []string{AbilityRoot}, scope: scope(store.Visibility{}, nil)},
		{name: "read-only-token", abilities: []string{AbilityRead}, scope: scope(store.Visibility{}, nil)},
		{name: "no-read", abilities: []string{AbilityDeploy}, scope: scope(store.Visibility{}, nil)},
		{name: "deny-app", abilities: []string{AbilityRead}, policies: []store.Policy{diffPolicy("Deny", resourcePrefixApp+app(), resourcePrefixApp+app())}, scope: scope(store.Visibility{}, nil)},
		{name: "deny-wildcard", abilities: []string{AbilityRead}, policies: []store.Policy{diffPolicy("Deny", resourcePrefixApp+"app-1*")}, scope: scope(store.Visibility{}, nil)},
		{name: "allow-env", abilities: []string{AbilityWrite}, policies: []store.Policy{diffPolicy("Allow", resourcePrefixEnvironment+env.ID)}, scope: scope(store.Visibility{}, nil)},
		{name: "env-restricted", abilities: []string{AbilityRead}, scope: scope(restricted, nil)},
		{name: "single-team", abilities: []string{AbilityRead, AbilityWrite}, scope: scope(store.Visibility{}, &teamScope{roles: map[string]string{team(): roles[rng.Intn(len(roles))]}})},
		{name: "multi-team", abilities: []string{AbilityRead}, scope: scope(store.Visibility{}, &teamScope{roles: map[string]string{team(): store.TeamRoleViewer, team(): store.TeamRoleDeveloper}, instance: rng.Intn(2) == 0})},
		{name: "org-owner", abilities: []string{AbilityRoot}, scope: scope(store.Visibility{}, &teamScope{roles: map[string]string{team(): store.TeamRoleOwner}})},
		{name: "project-scoped-token", abilities: []string{AbilityRead}, policies: []store.Policy{diffPolicy("Deny", resourcePrefixApp+app())}, scope: scope(restricted, &teamScope{roles: map[string]string{team(): store.TeamRoleViewer}, projectID: fx.projects[rng.Intn(len(fx.projects))]})},
		{name: "all-teams-admin", abilities: []string{AbilityRead}, scope: scope(store.Visibility{}, &teamScope{roles: map[string]string{store.DefaultTeamID: store.TeamRoleAdmin}, allTeams: true, projectID: fx.projects[0]})},
	}
}

func diffFilters(rng *rand.Rand, fx diffFixture) []store.AppListFilter {
	queries := []string{"", "1", "web", "APP-0", "%", "_"}
	out := []store.AppListFilter{{}}
	for i := 0; i < 6; i++ {
		f := store.AppListFilter{Query: queries[rng.Intn(len(queries))]}
		switch rng.Intn(4) {
		case 0:
			f.ProjectID = fx.projects[rng.Intn(len(fx.projects))]
		case 1:
			e := fx.envs[rng.Intn(len(fx.envs))]
			f.Environment = []string{e.ID, e.Name}[rng.Intn(2)]
		case 2:
			f.Environments = []string{fx.envs[rng.Intn(len(fx.envs))].Name, "production"}
		}
		if rng.Intn(3) == 0 {
			f.Tags, f.AnyTag = []string{fx.tags[rng.Intn(3)], fx.tags[rng.Intn(3)]}, rng.Intn(2) == 0
		}
		out = append(out, f)
	}
	return out
}

// TestListVisibleAppsPagedMatchesOracle runs the names-first path and the old load-all path over random fixtures.
func TestListVisibleAppsPagedMatchesOracle(t *testing.T) {
	ctx := context.Background()
	for seed := int64(1); seed <= 12; seed++ {
		rng := rand.New(rand.NewSource(seed)) // #nosec G404 -- seeded fixtures must be reproducible
		_, db := newTestRouter(t)
		fx := seedDiffFixture(ctx, t, rng, db)
		callers := diffCallers(ctx, t, rng, fx)
		for _, f := range diffFilters(rng, fx) {
			for _, c := range callers {
				canRead := func(app string) bool {
					return c.scope.authorize(c.abilities, c.policies, AbilityRead, resourcePrefixApp, app)
				}
				_, full, err := oracleListVisibleApps(ctx, db, f, canRead)
				if err != nil {
					t.Fatal(err)
				}
				for limit := 0; limit <= 7; limit++ {
					step := max(limit, 1)
					for offset := rng.Intn(2); offset <= full+1; offset += step {
						pf := f
						pf.Limit, pf.Offset = limit, offset
						want, wantTotal, err := oracleListVisibleApps(ctx, db, pf, canRead)
						if err != nil {
							t.Fatal(err)
						}
						got, gotTotal, err := listVisibleAppsPaged(ctx, db, pf, canRead)
						if err != nil {
							t.Fatal(err)
						}
						if gotTotal != wantTotal || !reflect.DeepEqual(got, want) {
							t.Fatalf("seed %d caller %s filter %+v: total %d want %d, got %v want %v", seed, c.name, pf, gotTotal, wantTotal, pagingDiffNames(got), pagingDiffNames(want))
						}
						if limit == 0 {
							break
						}
					}
				}
			}
		}
	}
}

func pagingDiffNames(svcs []store.DesiredService) []string {
	out := make([]string, len(svcs))
	for i, s := range svcs {
		out[i] = s.Name
	}
	return out
}

// TestAppsListScopedTokenPagesThroughHandler walks every page size end to end for a real deny-policy token.
func TestAppsListScopedTokenPagesThroughHandler(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie, err := loginTestSessionSlow(rt, db)
	if err != nil {
		t.Fatal(err)
	}
	const n = 23
	benchSeedApps(t, db, n)
	token := benchScopedToken(t, rt, cookie)
	h := rt.Handler()
	for limit := 1; limit <= 7; limit++ {
		var seen []string
		for offset := 0; offset < n+limit; offset += limit {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, benchBearer(http.MethodGet, fmt.Sprintf("/api/v1/apps?limit=%d&offset=%d", limit, offset), token))
			var page []appListResource
			mustDecodeBench(t, rec, http.StatusOK, &page)
			if got := rec.Header().Get("X-Total-Count"); got != strconv.Itoa(n-1) {
				t.Fatalf("limit %d offset %d: total %s, want %d", limit, offset, got, n-1)
			}
			for _, a := range page {
				seen = append(seen, a.Name)
			}
		}
		if len(seen) != n-1 || seen[0] != "app-00001" || seen[len(seen)-1] != fmt.Sprintf("app-%05d", n-1) {
			t.Fatalf("limit %d: pages %v", limit, seen)
		}
	}
}
