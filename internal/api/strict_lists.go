package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// strictView is what a caller may see on instance wide lists while strict
// isolation is on. A nil view means no restriction applies.
type strictView struct {
	canApp func(string) bool
	canDB  func(string) bool
	// instanceWide is true for callers outside every team scope (root), even
	// while the switcher narrows which resources they see.
	instanceWide bool
}

// strictViewFor returns nil when strict isolation is off or the caller sees
// everything, so instance lists can filter with one nil check.
func (rt *Router) strictViewFor(r *http.Request) (*strictView, error) {
	if !rt.strictOrgIsolation(r.Context()) {
		return nil, nil
	}
	abilities, policies, scope, err := rt.callerScope(r)
	if err != nil {
		return nil, err
	}
	if len(policies) == 0 && !scope.restricted() {
		return nil, nil
	}
	return &strictView{
		canApp: func(n string) bool {
			return scope.authorize(abilities, policies, AbilityRead, resourcePrefixApp, n)
		},
		canDB: func(n string) bool {
			return scope.authorize(abilities, policies, AbilityRead, resourcePrefixDatabase, n)
		},
		instanceWide: scope.teams == nil || hasAbility(abilities, AbilityRoot),
	}, nil
}

// hiddenNames lists the apps, databases and domains the view cannot see, the
// names a free text message must not carry.
func (rt *Router) strictHiddenNames(r *http.Request, v *strictView) ([]string, error) {
	var hidden []string
	svcs, err := rt.apps.ListDesiredServices(r.Context())
	if err != nil {
		return nil, fmt.Errorf("strict view: list apps: %w", err)
	}
	for _, s := range svcs {
		if !v.canApp(s.Name) {
			hidden = append(hidden, s.Name)
			hidden = append(hidden, s.Domains...)
		}
	}
	dbs, err := rt.databases.ListDesiredDatabases(r.Context())
	if err != nil {
		return nil, fmt.Errorf("strict view: list databases: %w", err)
	}
	for _, d := range dbs {
		if !v.canDB(d.Name) {
			hidden = append(hidden, d.Name)
		}
	}
	owners, err := rt.domainOwners(r.Context())
	if err != nil {
		return nil, err
	}
	for domain := range owners {
		if !certVisible(domain, nil, owners, v.canApp) {
			hidden = append(hidden, domain)
		}
	}
	return hidden, nil
}

// doctorFilter hides checks that name something the caller cannot see. The
// hidden names are computed once so a stream filters every event cheaply.
type doctorFilter struct{ names []string }

// doctorFilterFor returns nil when no restriction applies to the caller.
func (rt *Router) doctorFilterFor(r *http.Request) (*doctorFilter, error) {
	v, err := rt.strictViewFor(r)
	if err != nil || v == nil {
		return nil, err
	}
	hidden, err := rt.strictHiddenNames(r, v)
	if err != nil {
		return nil, err
	}
	return &doctorFilter{names: hidden}, nil
}

// keep returns the checks that name nothing hidden. A nil filter keeps all.
func (f *doctorFilter) keep(in []doctorCheckResource) []doctorCheckResource {
	if f == nil {
		return in
	}
	out := make([]doctorCheckResource, 0, len(in))
	for _, c := range in {
		if !f.hides(c) {
			out = append(out, c)
		}
	}
	return out
}

// hides is true when the check text names a hidden resource. A hidden name
// that is also a word of the check's own title or code (an app called "disk"
// next to the disk check) cannot tell the two apart, so it never hides it.
func (f *doctorFilter) hides(c doctorCheckResource) bool {
	own := textTokens(c.Name + " " + c.Code)
	text := strings.ToLower(c.Name + "\n" + c.Message + "\n" + c.Fix + "\n" + c.Evidence)
	for _, n := range f.names {
		n = strings.ToLower(strings.TrimSpace(n))
		if n == "" || (!strings.Contains(n, ".") && own[n]) {
			continue
		}
		if mentionsName(text, n) {
			return true
		}
	}
	return false
}

func textTokens(s string) map[string]bool {
	out := map[string]bool{}
	for _, t := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !isNameRune(r) }) {
		out[t] = true
	}
	return out
}

func isNameRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
}

// mentionsName finds a lowercase name in lowercase text as a whole word.
// Short names (under four characters) also refuse neighbors that glue
// words together, such as a dash, underscore or dot.
func mentionsName(text, name string) bool {
	const shortName = 4
	glue := func(r byte) bool {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			return true
		}
		return len(name) < shortName && r == '.'
	}
	for from := 0; from < len(text); {
		i := strings.Index(text[from:], name)
		if i < 0 {
			return false
		}
		start, end := from+i, from+i+len(name)
		if (start == 0 || !glue(text[start-1])) && (end == len(text) || !glue(text[end])) {
			return true
		}
		from = start + 1
	}
	return false
}

// filterDoctorChecks drops checks whose text names something the caller
// cannot see. The overall ok flag is kept, it names nothing.
func (rt *Router) filterDoctorChecks(r *http.Request, resp systemDoctorResponse) (systemDoctorResponse, error) {
	f, err := rt.doctorFilterFor(r)
	if err != nil {
		return resp, err
	}
	resp.Checks = f.keep(resp.Checks)
	return resp, nil
}

func mentionsAny(lowerText string, names []string) bool {
	for _, n := range names {
		if n = strings.ToLower(strings.TrimSpace(n)); n != "" && mentionsName(lowerText, n) {
			return true
		}
	}
	return false
}

// filterSilences keeps a silence only when its issue key names an app or
// database the caller can see and none it cannot. Keys naming nothing known
// are instance wide and stay root only.
func (rt *Router) filterSilences(r *http.Request, in []store.Silence) ([]store.Silence, error) {
	v, err := rt.strictViewFor(r)
	if err != nil || v == nil {
		return in, err
	}
	svcs, err := rt.apps.ListDesiredServices(r.Context())
	if err != nil {
		return nil, fmt.Errorf("filter silences: list apps: %w", err)
	}
	dbs, err := rt.databases.ListDesiredDatabases(r.Context())
	if err != nil {
		return nil, fmt.Errorf("filter silences: list databases: %w", err)
	}
	known := map[string]bool{}
	for _, s := range svcs {
		known[s.Name] = v.canApp(s.Name)
	}
	for _, d := range dbs {
		if _, dup := known[d.Name]; !dup {
			known[d.Name] = v.canDB(d.Name)
		}
	}
	hidden, err := rt.strictHiddenNames(r, v)
	if err != nil {
		return nil, err
	}
	out := make([]store.Silence, 0, len(in))
	for _, s := range in {
		if mentionsAny(strings.ToLower(s.Scope+"\n"+s.Reason), hidden) {
			continue
		}
		seen, allowed := false, true
		for _, seg := range strings.Split(s.IssueKey, ":") {
			if ok, isKnown := known[seg]; isKnown {
				seen = true
				allowed = allowed && ok
			}
		}
		if seen && allowed {
			out = append(out, s)
		}
	}
	return out, nil
}
