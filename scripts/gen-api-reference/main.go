// Command gen-api-reference regenerates the route tables in docs/api-reference.md
// from the mux registrations in internal/api/routes*.go.
//
// Usage (from the repo root): go run ./scripts/gen-api-reference
// Pass -check to exit non-zero instead of writing when the doc is stale.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type route struct{ method, path, ability, handler string }

func (r route) key() string { return r.method + " " + r.path }

var (
	regFull = regexp.MustCompile(`mux\.HandleFunc\("([A-Z]+) ([^"]+)",\s*(.+)\)\s*$`)
	regAbil = regexp.MustCompile(`^rt\.requireAbility(?:ForResource)?\((Ability\w+),`)
	regAuth = regexp.MustCompile(`^rt\.requireAuth\(`)
	regHndl = regexp.MustCompile(`(handle\w+)`)
	regCnt  = regexp.MustCompile(`\d+ endpoints`)
)

func parseRoutes(dir string) ([]route, error) {
	files, err := filepath.Glob(filepath.Join(dir, "routes*.go"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	var out []route
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f) //nolint:gosec // developer tool reading repo files
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(string(b), "\n") {
			m := regFull.FindStringSubmatch(strings.TrimSpace(line))
			if m == nil {
				continue
			}
			r := route{method: m[1], path: m[2], ability: "Public"}
			if a := regAbil.FindStringSubmatch(m[3]); a != nil {
				r.ability = a[1]
			} else if regAuth.MatchString(m[3]) {
				r.ability = "Session"
			}
			if h := regHndl.FindAllString(m[3], -1); h != nil {
				r.handler = h[len(h)-1]
			}
			out = append(out, r)
		}
	}
	return out, nil
}

func segs(p string) []string { return strings.Split(strings.Trim(p, "/"), "/") }

func common(a, b string) int {
	x, y := segs(a), segs(b)
	n := 0
	for n < len(x) && n < len(y) && x[n] == y[n] {
		n++
	}
	return n
}

func isRow(l string) bool { return strings.HasPrefix(l, "|") }

type section struct {
	name string
	rows []route
}

func generate(doc string, routes []route) string {
	lines := strings.Split(doc, "\n")
	var secs []*section
	idx := map[string]*section{}
	existing := map[string]bool{}
	cur := ""
	for _, l := range lines {
		if strings.HasPrefix(l, "## ") {
			cur = l
			continue
		}
		if !isRow(l) || strings.HasPrefix(l, "| Method") || strings.HasPrefix(l, "| ---") {
			continue
		}
		c := strings.Split(strings.Trim(l, "| "), "|")
		if len(c) != 4 {
			continue
		}
		r := route{strings.TrimSpace(c[0]), strings.TrimSpace(c[1]), strings.TrimSpace(c[2]), strings.TrimSpace(c[3])}
		s := idx[cur]
		if s == nil {
			s = &section{name: cur}
			idx[cur] = s
			secs = append(secs, s)
		}
		s.rows = append(s.rows, r)
		existing[r.key()] = true
	}
	live := map[string]route{}
	for _, r := range routes {
		live[r.key()] = r
	}
	// Keep live rows in place (refreshing ability and handler), drop stale ones.
	for _, s := range secs {
		var kept []route
		for _, r := range s.rows {
			if lr, ok := live[r.key()]; ok {
				if lr.ability == "Public" && strings.HasPrefix(r.ability, "Public") {
					lr.ability = r.ability
				}
				kept = append(kept, lr)
			}
		}
		s.rows = kept
	}
	// Place new routes in the group whose rows share the longest path prefix.
	other := &section{name: "## Other"}
	otherIsNew := idx["## Other"] == nil
	if !otherIsNew {
		other = idx["## Other"]
	}
	for _, r := range routes {
		if existing[r.key()] {
			continue
		}
		var best *section
		bestScore := 2
		for _, s := range secs {
			for _, e := range s.rows {
				if sc := common(e.path, r.path); sc > bestScore {
					best, bestScore = s, sc
				}
			}
		}
		if best == nil {
			best = other
		}
		best.rows = append(best.rows, r)
	}
	if otherIsNew && len(other.rows) > 0 {
		idx["## Other"] = other
	}
	var out []string
	cur = ""
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if strings.HasPrefix(l, "## ") {
			cur = l
			if l == "## See also" && otherIsNew && idx["## Other"] == other {
				out = append(out, "## Other", "", "Routes that do not fit an existing group.", "")
				out = append(out, table(other.rows)...)
				out = append(out, "")
			}
		}
		if strings.HasPrefix(l, "::: details") {
			if s := idx[cur]; s != nil {
				l = regCnt.ReplaceAllString(l, fmt.Sprintf("%d endpoints", len(s.rows)))
			}
		}
		if strings.HasPrefix(l, "| Method") {
			for i < len(lines) && isRow(lines[i]) {
				i++
			}
			i--
			if s := idx[cur]; s != nil {
				out = append(out, table(s.rows)...)
			}
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

func table(rows []route) []string {
	out := []string{"| Method | Path | Ability | Handler |", "| --- | --- | --- | --- |"}
	for _, r := range rows {
		out = append(out, fmt.Sprintf("| %s | %s | %s | %s |", r.method, r.path, r.ability, r.handler))
	}
	return out
}

func main() {
	check := flag.Bool("check", false, "fail if the doc is stale instead of writing it")
	docPath := flag.String("doc", "docs/api-reference.md", "doc to update")
	apiDir := flag.String("api", "internal/api", "directory holding routes*.go")
	flag.Parse()
	routes, err := parseRoutes(*apiDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	b, err := os.ReadFile(*docPath) //nolint:gosec // developer tool
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	got := generate(string(b), routes)
	if got == string(b) {
		fmt.Printf("%s up to date (%d routes)\n", *docPath, len(routes))
		return
	}
	if *check {
		fmt.Fprintf(os.Stderr, "%s is stale: run go run ./scripts/gen-api-reference\n", *docPath)
		os.Exit(1)
	}
	if err := os.WriteFile(*docPath, []byte(got), 0o644); err != nil { //nolint:gosec // docs file
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%s updated (%d routes)\n", *docPath, len(routes))
}
