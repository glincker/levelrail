package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/configtranslate"
)

// appCreatableFragment drops what apps create cannot apply (databases: and
// env from: refs) so the emitted file is accepted as-is. The copy leaves the
// original intact for the printed command sequence.
func appCreatableFragment(f configtranslate.Fragment) configtranslate.Fragment {
	out := configtranslate.Fragment{Version: f.Version, Services: map[string]configtranslate.Service{}}
	for k, svc := range f.Services {
		env := map[string]configtranslate.Env{}
		for ek, ev := range svc.Env {
			if ev.From == "" {
				env[ek] = ev
			}
		}
		svc.Env = env
		out.Services[k] = svc
	}
	return out
}

// refDatabase returns the database name in a ref such as postgres.main.url.
func refDatabase(ref string) string {
	parts := strings.Split(ref, ".")
	if len(parts) >= 2 {
		return parts[1]
	}
	return ref
}

// importConfigSteps orders the commands: databases first, then the apps, then
// one databases link per former from: reference so each connection variable lands.
func importConfigSteps(f configtranslate.Fragment, prog, specFile string) []string {
	var out []string
	for _, k := range sortedFragmentKeys(f.Databases) {
		if !configtranslate.ValidDatabaseName(k) {
			out = append(out, configtranslate.SkippedStep("database", k))
			continue
		}
		out = append(out, fmt.Sprintf("%s databases create --name %s --engine %s --version VERSION", prog, k, configtranslate.ShellQuote(f.Databases[k].Engine)))
	}
	out = append(out, configtranslate.NextSteps(configtranslate.Fragment{Version: f.Version, Services: f.Services}, prog, specFile)...)
	for _, k := range sortedFragmentKeys(f.Services) {
		seen := map[string]bool{}
		var dbs []string
		for _, ev := range f.Services[k].Env {
			if ev.From != "" && !seen[refDatabase(ev.From)] {
				seen[refDatabase(ev.From)] = true
				dbs = append(dbs, refDatabase(ev.From))
			}
		}
		sort.Strings(dbs)
		for _, db := range dbs {
			if !configtranslate.ValidDatabaseName(db) {
				out = append(out, configtranslate.SkippedStep("database", db))
				continue
			}
			out = append(out, fmt.Sprintf("%s databases link %s APP   # APP is the --name given to the %s service", prog, db, strconv.Quote(k)))
		}
	}
	return out
}

func sortedFragmentKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
