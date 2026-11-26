package api

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/dbauthz"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDatabaseAuthzAbilityConstantsMatch(t *testing.T) {
	if dbauthz.EffectiveAbility(dbauthz.ActionRead) != AbilityReadSensitive || dbauthz.EffectiveAbility(dbauthz.ActionAdmin) != AbilityRoot {
		t.Fatal("dbauthz ability mapping drifted from the API abilities")
	}
	for _, a := range dbauthz.Actions {
		if !isKnownAbility(string(a)) {
			t.Fatalf("%s is not in the IAM known set", a)
		}
	}
}

// explorerHandlers is the exact set of routes that must go through dbauthz.Resolve.
var explorerHandlers = []string{
	"handleGetDatabaseSchema", "handleGetDatabaseTableRows", "handleGetDatabaseTableStructure",
	"handleDatabaseQuery", "handleDatabaseQueryWrite", "handleDatabaseExplain",
	"handleGetDatabaseExplorerCapabilities", "handleScanDatabaseKeys", "handleGetDatabaseKey",
	"handleGetMongoCollections", "handleMongoFind", "handleListDatabaseQueryHistory",
	"handleClearDatabaseQueryHistory", "handleListDatabaseSavedQueries", "handleSaveDatabaseQuery",
	"handleDeleteDatabaseSavedQuery", "handleGetDatabaseEngineStats",
	"handleGetDynamoTables", "handleDynamoQuery", "handleDynamoScan",
	"handleGetRedisInfo", "handleGetRedisSlowlog", "handleGetRedisClients",
	"handleGetDatabaseAdvisor",
}

// TestDatabaseExplorerRoutesUseResolve fails when an explorer route loses the
// dbauthz gate, or when a new handler in the viewer files is not listed.
func TestDatabaseExplorerRoutesUseResolve(t *testing.T) {
	handlerRe := regexp.MustCompile(`(?m)^func \(rt \*Router\) (handle\w+)\(`)
	listed := map[string]bool{}
	for _, h := range explorerHandlers {
		listed[h] = true
	}
	viewerFiles, _ := filepath.Glob("database_viewer*.go")
	for _, f := range viewerFiles {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f) //nolint:gosec // package dir glob
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range handlerRe.FindAllStringSubmatch(string(src), -1) {
			if !listed[m[1]] {
				t.Errorf("%s in %s is not in explorerHandlers; list it and gate it with requireAbilityForDatabase", m[1], f)
			}
		}
	}
	routeFiles, _ := filepath.Glob("routes*.go")
	count := map[string]int{}
	for _, f := range routeFiles {
		src, err := os.ReadFile(f) //nolint:gosec // package dir glob
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(src), "\n") {
			code, _, _ := strings.Cut(line, "//")
			for _, h := range explorerHandlers {
				if !strings.Contains(code, "mux.HandleFunc(") || !strings.Contains(code, "rt."+h+")") {
					continue
				}
				count[h]++
				if !regexp.MustCompile(`rt\.requireAbilityForDatabase\(\w+, dbauthz\.Action\w+, rt\.` + h + `\)\)\s*$`).MatchString(code) {
					t.Errorf("%s is registered without dbauthz.Resolve: %s", h, strings.TrimSpace(code))
				}
			}
		}
	}
	for _, h := range explorerHandlers {
		if count[h] != 1 {
			t.Errorf("%s is registered %d times, want exactly 1", h, count[h])
		}
	}
}

func dbPolicies(docs ...string) []store.Policy {
	out := make([]store.Policy, 0, len(docs))
	for i, d := range docs {
		out = append(out, store.Policy{ID: fmt.Sprintf("p%d", i), Document: d})
	}
	return out
}

// realEngine runs the real authorizeResource rules with no team or environment data.
type realEngine struct{ pols []store.Policy }

func (e realEngine) Evaluate(_ context.Context, c dbauthz.Caller, ability string, action dbauthz.Action, resource string) (dbauthz.Verdict, error) {
	for _, n := range []string{ability, string(action)} {
		if m, ok := evaluatePolicies(e.pols, n, resource); m && !ok {
			return dbauthz.Verdict{Denied: true}, nil
		}
	}
	if authorizeResource(c.Abilities, e.pols, ability, resource) {
		return dbauthz.Verdict{Allowed: true}, nil
	}
	by := authorizeResource(nil, e.pols, string(action), resource)
	return dbauthz.Verdict{Allowed: by, ByAction: by}, nil
}

func TestDatabaseAuthzTemplateDeniesHold(t *testing.T) {
	denyRoot := `{"Statement":[{"Effect":"Deny","Action":["root"],"Resource":["database:prod"]}]}`
	denyRS := `{"Statement":[{"Effect":"Deny","Action":["read:sensitive"],"Resource":["database:*"]}]}`
	allowWrite := `{"Statement":[{"Effect":"Allow","Action":["db:write"],"Resource":["database:*"]}]}`
	denyDBRead := `{"Statement":[{"Effect":"Deny","Action":["db:read"],"Resource":["database:prod"]}]}`
	tests := []struct {
		name   string
		docs   []string
		abil   []string
		action dbauthz.Action
		want   bool
	}{
		{"root caller, Deny root, write", []string{denyRoot}, []string{AbilityRoot}, dbauthz.ActionWrite, false},
		{"root caller, Deny root, read", []string{denyRoot}, []string{AbilityRoot}, dbauthz.ActionRead, true},
		{"Allow db:write cannot beat Deny root", []string{denyRoot, allowWrite}, []string{AbilityRead}, dbauthz.ActionWrite, false},
		{"Deny read:sensitive on database:* denies reads for root", []string{denyRS}, []string{AbilityRoot}, dbauthz.ActionRead, false},
		{"Deny read:sensitive denies query for root", []string{denyRS}, []string{AbilityRoot}, dbauthz.ActionQuery, false},
		{"Deny db:read beats root", []string{denyDBRead}, []string{AbilityRoot}, dbauthz.ActionRead, false},
		{"Allow db:write grants a reader", []string{allowWrite}, []string{AbilityRead}, dbauthz.ActionWrite, true},
		{"no grant", nil, []string{AbilityRead}, dbauthz.ActionWrite, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &dbauthz.Resolver{Engine: realEngine{pols: dbPolicies(tt.docs...)}}
			d, err := r.Resolve(context.Background(), dbauthz.Caller{Abilities: tt.abil}, "database:prod", tt.action)
			if err != nil || d.Allowed != tt.want {
				t.Fatalf("allowed=%v want %v reason=%q err=%v", d.Allowed, tt.want, d.Reason, err)
			}
		})
	}
}
