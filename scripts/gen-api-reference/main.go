// Command gen-api-reference regenerates the route tables in docs/api-reference.md
// and the route metadata in internal/api/openapi_gen.go, both derived from
// the mux registrations in internal/api/routes*.go.
//
// Usage (from the repo root): go run ./scripts/gen-api-reference
// Pass -check to exit non-zero instead of writing when any output is stale.
// It also writes docs/public/openapi.json, an OpenAPI 3.1 document.
package main

import (
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type route struct{ method, path, ability, handler, description string }

func (r route) key() string { return r.method + " " + r.path }

var (
	regFull    = regexp.MustCompile(`mux\.HandleFunc\("([A-Z]+) ([^"]+)",\s*(.+)\)\s*$`)
	regAbil    = regexp.MustCompile(`^rt\.requireAbility(?:ForResource)?\((Ability\w+),`)
	regAuth    = regexp.MustCompile(`^rt\.(?:requireAuth|sessionOrRootToken)\(`)
	regHndl    = regexp.MustCompile(`(handle\w+)`)
	regCnt     = regexp.MustCompile(`\d+ endpoints`)
	regComment = regexp.MustCompile(`^//\s?(.*)$`)
)

// parseRoutes walks every routes*.go file (excluding tests) and returns
// one route per registered mux.HandleFunc, including the plain-English
// description taken from the unbroken block of // comment lines directly
// above it, if any. A comment shared above several routes (e.g. one
// rationale covering three sibling endpoints) only attaches to the first
// of them, the same honest gap the API explorer's description column
// surfaces rather than hides.
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
		lines := strings.Split(string(b), "\n")
		for i, line := range lines {
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
			r.description = precedingComment(lines, i)
			out = append(out, r)
		}
	}
	return out, nil
}

// precedingComment collects the contiguous run of // comment lines
// immediately above lines[idx], stopping at the first blank or
// non-comment line, and joins them into one sentence-ish string.
func precedingComment(lines []string, idx int) string {
	var collected []string
	for i := idx - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if l == "" {
			break
		}
		m := regComment.FindStringSubmatch(l)
		if m == nil {
			break
		}
		collected = append(collected, m[1])
	}
	if len(collected) == 0 {
		return ""
	}
	for i, j := 0, len(collected)-1; i < j; i, j = i+1, j-1 {
		collected[i], collected[j] = collected[j], collected[i]
	}
	return strings.TrimSpace(strings.Join(collected, " "))
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

// parseDocSections reads the ## headings and their route tables out of
// docs/api-reference.md, used both to regenerate that doc and to assign
// every route a "group" (the heading it currently lives under) for the
// API explorer's JSON.
func parseDocSections(doc string) (secs []*section, idx map[string]*section, existing map[string]bool) {
	idx = map[string]*section{}
	existing = map[string]bool{}
	cur := ""
	for _, l := range strings.Split(doc, "\n") {
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
		r := route{method: strings.TrimSpace(c[0]), path: strings.TrimSpace(c[1]), ability: strings.TrimSpace(c[2]), handler: strings.TrimSpace(c[3])}
		s := idx[cur]
		if s == nil {
			s = &section{name: cur}
			idx[cur] = s
			secs = append(secs, s)
		}
		s.rows = append(s.rows, r)
		existing[r.key()] = true
	}
	return secs, idx, existing
}

// groupsFor assigns every live route the heading (without "## ") it sits
// under in the doc, placing a route the doc doesn't mention yet into
// whichever existing group shares the longest path prefix, or "Other".
func groupsFor(doc string, routes []route) map[string]string {
	secs, _, existing := parseDocSections(doc)
	groups := map[string]string{}
	for _, s := range secs {
		for _, r := range s.rows {
			groups[r.key()] = strings.TrimPrefix(s.name, "## ")
		}
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
		if best != nil {
			groups[r.key()] = strings.TrimPrefix(best.name, "## ")
		} else {
			groups[r.key()] = "Other"
		}
	}
	return groups
}

func generate(doc string, routes []route) string {
	lines := strings.Split(doc, "\n")
	secs, idx, existing := parseDocSections(doc)
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
	cur := ""
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

// goStringLit renders s as a double-quoted Go string literal, escaping
// the handful of characters that matter (quotes, backslashes); route
// descriptions are plain prose pulled from source comments, never
// containing anything more exotic than that.
func goStringLit(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// genOpenAPIFile renders internal/api/openapi_gen.go: a committed,
// generated Go literal of every route's method/path/ability/group/handler
// and (when one exists) its source-comment description. It exists so the
// GET /api/v1/openapi.json handler (internal/api/openapi.go) can serve
// this data at runtime from the compiled binary, with no source tree or
// docs/ directory available on a deployed node to parse at request time.
func genOpenAPIFile(routes []route, groups map[string]string) (string, error) {
	sorted := make([]route, len(routes))
	copy(sorted, routes)
	sort.Slice(sorted, func(i, j int) bool {
		gi, gj := groups[sorted[i].key()], groups[sorted[j].key()]
		if gi != gj {
			return gi < gj
		}
		if sorted[i].path != sorted[j].path {
			return sorted[i].path < sorted[j].path
		}
		return sorted[i].method < sorted[j].method
	})
	var b strings.Builder
	b.WriteString("// Code generated by scripts/gen-api-reference; DO NOT EDIT.\n\n")
	b.WriteString("package api\n\n")
	b.WriteString("// openAPIRoute is one registered route, as mirrored into docs/api-reference.md\n")
	b.WriteString("// by the same generator. See internal/api/openapi.go for the handler that\n")
	b.WriteString("// serves this table as GET /api/v1/openapi.json.\n")
	b.WriteString("type openAPIRoute struct {\n")
	b.WriteString("\tMethod      string\n\tPath        string\n\tAbility     string\n\tGroup       string\n\tHandler     string\n\tDescription string\n}\n\n")
	fmt.Fprintf(&b, "// openAPIRoutes holds all %d routes known to scripts/gen-api-reference at\n", len(sorted))
	b.WriteString("// generation time. Run `go run ./scripts/gen-api-reference` after changing\n")
	b.WriteString("// any routes*.go registration and commit the result.\n")
	b.WriteString("var openAPIRoutes = []openAPIRoute{\n")
	for _, r := range sorted {
		fmt.Fprintf(&b,
			"\t{Method: %s, Path: %s, Ability: %s, Group: %s, Handler: %s, Description: %s},\n",
			goStringLit(r.method), goStringLit(r.path), goStringLit(r.ability),
			goStringLit(groups[r.key()]), goStringLit(r.handler), goStringLit(r.description),
		)
	}
	b.WriteString("}\n")
	formatted, err := format.Source([]byte(b.String()))
	if err != nil {
		return "", fmt.Errorf("format openapi_gen.go: %w", err)
	}
	return string(formatted), nil
}

func main() {
	check := flag.Bool("check", false, "fail if either output is stale instead of writing it")
	docPath := flag.String("doc", "docs/api-reference.md", "doc to update")
	apiDir := flag.String("api", "internal/api", "directory holding routes*.go")
	openAPIPath := flag.String("openapi", "internal/api/openapi_gen.go", "generated route metadata file to update")
	specPath := flag.String("spec", "docs/public/openapi.json", "OpenAPI 3.1 document to update")
	brandPath := flag.String("brand", "brand.yaml", "brand file read for the spec title and contact")
	flag.Parse()

	routes, err := parseRoutes(*apiDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	docBytes, err := os.ReadFile(*docPath) //nolint:gosec // developer tool
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	doc := string(docBytes)
	gotDoc := generate(doc, routes)

	groups := groupsFor(doc, routes)
	gotOpenAPI, err := genOpenAPIFile(routes, groups)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	existingOpenAPI, err := os.ReadFile(*openAPIPath) //nolint:gosec // developer tool
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	brand, err := loadBrand(*brandPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	gotSpec, err := genOpenAPI31(routes, groups, brand)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	existingSpec, err := os.ReadFile(*specPath) //nolint:gosec // developer tool
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	docStale := gotDoc != doc
	openAPIStale := gotOpenAPI != string(existingOpenAPI)
	specStale := string(gotSpec) != string(existingSpec)

	if !docStale && !openAPIStale && !specStale {
		fmt.Printf("%s, %s and %s up to date (%d routes)\n", *docPath, *openAPIPath, *specPath, len(routes))
		return
	}
	if *check {
		if docStale {
			fmt.Fprintf(os.Stderr, "%s is stale: run go run ./scripts/gen-api-reference\n", *docPath)
		}
		if openAPIStale {
			fmt.Fprintf(os.Stderr, "%s is stale: run go run ./scripts/gen-api-reference\n", *openAPIPath)
		}
		if specStale {
			fmt.Fprintf(os.Stderr, "%s is stale: run go run ./scripts/gen-api-reference\n", *specPath)
		}
		os.Exit(1)
	}
	if docStale {
		if err := os.WriteFile(*docPath, []byte(gotDoc), 0o644); err != nil { //nolint:gosec // docs file
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if openAPIStale {
		if err := os.WriteFile(*openAPIPath, []byte(gotOpenAPI), 0o644); err != nil { //nolint:gosec // generated source file
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if specStale {
		if err := os.WriteFile(*specPath, gotSpec, 0o644); err != nil { //nolint:gosec // published docs asset
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	fmt.Printf("%s, %s and %s updated (%d routes)\n", *docPath, *openAPIPath, *specPath, len(routes))
}
