package compose_test

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/compose"
)

// genericForm matches a non-SERVICE ${VAR<op>...} form or a "$$" escape, the only
// shapes the new pass may change.
var genericForm = regexp.MustCompile(`\$\$|\$\{(?:[A-RT-Za-z_][A-Za-z0-9_]*|S[A-Za-z0-9_]*)(?::?[-?+])`)

type resolveFn func(*compose.File, func(string, string, int) (string, error), func(string, string, string) error) (map[string][]string, []compose.UnresolvedVar, error)

func resolveDump(t *testing.T, body string, fn resolveFn) string {
	t.Helper()
	f, err := compose.Parse([]byte(body))
	if err != nil {
		return "parse-error"
	}
	var persisted []string
	// Length is dropped: which service names the first use of a shared key is map-order dependent.
	gen := func(kind, key string, _ int) (string, error) { return fmt.Sprintf("g-%s-%s", kind, key), nil }
	per := func(svc, k, v string) error { persisted = append(persisted, svc+"/"+k+"="+v); return nil }
	secret, unresolved, err := fn(f, gen, per)
	var b strings.Builder
	fmt.Fprintf(&b, "err=%v\n", err)
	names := make([]string, 0, len(f.Services))
	for n := range f.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		keys := make([]string, 0)
		for k, v := range f.Services[n].Environment {
			keys = append(keys, k+"="+v)
		}
		sort.Strings(keys)
		sec := append([]string(nil), secret[n]...)
		sort.Strings(sec)
		fmt.Fprintf(&b, "%s env=%q cmd=%q secret=%v\n", n, keys, []string(f.Services[n].Command), sec)
	}
	sort.Strings(persisted)
	fmt.Fprintf(&b, "persisted=%q unresolved=%v\n", persisted, unresolved)
	return b.String()
}

func TestResolveMagicVarsCatalogMatchesLegacy(t *testing.T) {
	same := 0
	for _, tpl := range catalog.Templates {
		if genericForm.MatchString(tpl.Compose) {
			continue
		}
		want := resolveDump(t, tpl.Compose, compose.ResolveMagicVarsLegacy)
		got := resolveDump(t, tpl.Compose, compose.ResolveMagicVars)
		if got != want {
			t.Errorf("template %s resolves differently:\nold:\n%s\nnew:\n%s", tpl.ID, want, got)
		}
		same++
	}
	if same < 40 {
		t.Fatalf("only %d templates compared, want at least 40", same)
	}
	t.Logf("compared %d templates", same)
}
