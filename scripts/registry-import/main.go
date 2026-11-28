// Command registry-import ranks apps from a local JSON or CSV list that the
// registry lacks and resolves a thesvg icon slug for each, no network.
// Usage: go run ./scripts/registry-import -list apps.json [-top 60] [-md out.md]
package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	defaultDist   = "web/node_modules/@thesvg/react/dist"
	distEnv       = "THESVG_DIST"
	minNearLen    = 6
	rankScoreBase = 1_000_000.0
	maxTail       = 6
)

// App is one row of the source list.
type App struct {
	Name     string
	Repo     string
	Category string
	Icon     string
	Score    float64
}

// Status says how a list row relates to the registry.
type Status string

const (
	statusMissing Status = "missing"
	statusCovered Status = "covered"
	statusNear    Status = "near"
)

// Row is one classified app.
type Row struct {
	App
	Status  Status
	Match   string
	Slug    string
	HasLogo bool
}

// Report is the full classified result.
type Report struct {
	Missing []Row
	Near    []Row
	Covered []Row
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("registry-import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	list := fs.String("list", "", "source list, .json or .csv (required)")
	reg := fs.String("registry", "registry", "registry directory (templates/, baseline-ids.txt, pending/)")
	dist := fs.String("dist", "", "thesvg package dist directory (default $"+distEnv+" or "+defaultDist+")")
	top := fs.Int("top", 60, "how many missing apps to print per section")
	mdOut := fs.String("md", "", "also write the report as markdown to this file")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *list == "" {
		_, _ = fmt.Fprintln(stderr, "registry-import: -list is required")
		return 2
	}
	apps, err := loadList(*list)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "registry-import:", err)
		return 2
	}
	known, err := loadRegistry(*reg)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "registry-import:", err)
		return 2
	}
	dir := *dist
	if dir == "" {
		dir = os.Getenv(distEnv)
	}
	if dir == "" {
		dir = defaultDist
	}
	icons, err := loadIcons(dir)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "registry-import:", err)
		return 2
	}
	rep := classify(apps, known, icons)
	text := render(rep, *top, len(apps))
	_, _ = fmt.Fprint(stdout, text)
	if *mdOut != "" {
		if err := os.WriteFile(*mdOut, []byte(text), 0o600); err != nil {
			_, _ = fmt.Fprintln(stderr, "registry-import:", err)
			return 2
		}
	}
	return 0
}

// loadList reads a JSON array (or {"apps": [...]}) or a CSV with a header row.
func loadList(path string) ([]App, error) {
	b, err := os.ReadFile(path) //nolint:gosec // path chosen by the operator running the tool
	if err != nil {
		return nil, err
	}
	var rows []map[string]string
	if strings.EqualFold(filepath.Ext(path), ".csv") {
		rows, err = csvRows(b)
	} else {
		rows, err = jsonRows(b)
	}
	if err != nil {
		return nil, err
	}
	apps := make([]App, 0, len(rows))
	for i, r := range rows {
		name := first(r, "name", "title", "app")
		if name == "" {
			continue
		}
		apps = append(apps, App{
			Name:     name,
			Repo:     first(r, "repo", "source", "url", "github"),
			Category: first(r, "category", "tags", "type"),
			Icon:     first(r, "icon", "logo"),
			Score:    score(r, i, len(rows)),
		})
	}
	if len(apps) == 0 {
		return nil, errors.New("no rows with a name in " + path)
	}
	return apps, nil
}

func csvRows(b []byte) ([]map[string]string, error) {
	recs, err := csv.NewReader(strings.NewReader(string(b))).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(recs) < 2 {
		return nil, errors.New("csv has no data rows")
	}
	head := make([]string, len(recs[0]))
	for i, h := range recs[0] {
		head[i] = strings.ToLower(strings.TrimSpace(h))
	}
	out := make([]map[string]string, 0, len(recs)-1)
	for _, rec := range recs[1:] {
		m := map[string]string{}
		for i, v := range rec {
			if i < len(head) {
				m[head[i]] = strings.TrimSpace(v)
			}
		}
		out = append(out, m)
	}
	return out, nil
}

func jsonRows(b []byte) ([]map[string]string, error) {
	var raw []map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		var wrap struct {
			Apps []map[string]json.RawMessage `json:"apps"`
		}
		if err2 := json.Unmarshal(b, &wrap); err2 != nil || len(wrap.Apps) == 0 {
			return nil, fmt.Errorf("json must be an array of objects or {\"apps\": [...]}: %w", err)
		}
		raw = wrap.Apps
	}
	out := make([]map[string]string, 0, len(raw))
	for _, r := range raw {
		m := map[string]string{}
		for k, v := range r {
			m[strings.ToLower(k)] = scalar(v)
		}
		out = append(out, m)
	}
	return out, nil
}

// scalar renders a JSON value as text: strings as is, numbers verbatim, arrays joined.
func scalar(v json.RawMessage) string {
	var s string
	if json.Unmarshal(v, &s) == nil {
		return strings.TrimSpace(s)
	}
	var arr []string
	if json.Unmarshal(v, &arr) == nil {
		return strings.Join(arr, ", ")
	}
	var n json.Number
	if json.Unmarshal(v, &n) == nil {
		return n.String()
	}
	return ""
}

func first(m map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(m[k]); v != "" {
			return v
		}
	}
	return ""
}

// score prefers stars, then a rank (lower is better), then list position.
func score(r map[string]string, idx, total int) float64 {
	for _, k := range []string{"stars", "stargazers", "popularity"} {
		if f, err := strconv.ParseFloat(strings.ReplaceAll(r[k], ",", ""), 64); err == nil && f > 0 {
			return f
		}
	}
	if f, err := strconv.ParseFloat(r["rank"], 64); err == nil && f > 0 {
		return rankScoreBase / f
	}
	return float64(total - idx)
}

// registryIndex maps normalized keys to the registry id that owns them.
type registryIndex map[string]string

func loadRegistry(root string) (registryIndex, error) {
	idx := registryIndex{}
	dirs, err := os.ReadDir(filepath.Join(root, "templates"))
	if err != nil {
		return nil, err
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		id := d.Name()
		idx.add(id, id)
		b, err := os.ReadFile(filepath.Join(root, "templates", id, "template.yaml")) //nolint:gosec // registry path from flag
		if err != nil {
			continue
		}
		var t struct {
			Name     string `yaml:"name"`
			Manifest struct {
				Aliases []string `yaml:"aliases"`
			} `yaml:"manifest"`
		}
		if yaml.Unmarshal(b, &t) != nil {
			continue
		}
		idx.add(t.Name, id)
		for _, a := range t.Manifest.Aliases {
			idx.add(a, id)
		}
	}
	if b, err := os.ReadFile(filepath.Join(root, "baseline-ids.txt")); err == nil { //nolint:gosec // registry path from flag
		for _, l := range strings.Split(string(b), "\n") {
			if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
				idx.add(l, l)
			}
		}
	}
	if pend, err := os.ReadDir(filepath.Join(root, "pending")); err == nil {
		for _, d := range pend {
			if d.IsDir() {
				idx.add(d.Name(), d.Name())
			}
		}
	}
	return idx, nil
}

func (r registryIndex) add(name, id string) {
	for _, k := range keys(name) {
		if _, ok := r[k]; !ok {
			r[k] = id
		}
	}
}

var noiseSuffixes = []string{"selfhosted", "server", "app", "web", "ui", "js", "io", "project"}

// norm lowercases and drops everything but letters and digits.
func norm(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// keys returns the normalized name plus the form without one noise suffix.
func keys(name string) []string {
	n := norm(name)
	if n == "" {
		return nil
	}
	out := []string{n}
	for _, suf := range noiseSuffixes {
		if strings.HasSuffix(n, suf) && len(n)-len(suf) >= 3 {
			out = append(out, strings.TrimSuffix(n, suf))
		}
	}
	return out
}

// find returns the registry id for an exact key hit, or a near one.
func (r registryIndex) find(name string) (id string, near bool) {
	ks := keys(name)
	for _, k := range ks {
		if v, ok := r[k]; ok {
			return v, false
		}
	}
	for _, k := range ks {
		if len(k) < minNearLen {
			continue
		}
		for rk, v := range r {
			if len(rk) >= minNearLen && (editWithin1(k, rk) || prefixClose(k, rk)) {
				return v, true
			}
		}
	}
	return "", false
}

// prefixClose flags one key extending the other by a short tail, like actual and actualbudget.
func prefixClose(a, b string) bool {
	if len(a) > len(b) {
		a, b = b, a
	}
	return len(a) >= minNearLen && strings.HasPrefix(b, a) && len(b)-len(a) <= maxTail
}

func editWithin1(a, b string) bool {
	if a == b {
		return true
	}
	if d := len(a) - len(b); d > 1 || d < -1 {
		return false
	}
	if len(a) < len(b) {
		a, b = b, a
	}
	i := 0
	for i < len(b) && a[i] == b[i] {
		i++
	}
	if len(a) == len(b) {
		return a[i+1:] == b[i+1:]
	}
	return a[i+1:] == b[i:]
}

// iconIndex maps normalized thesvg names to their real slugs.
type iconIndex struct {
	exact map[string]bool
	byKey map[string][]string
}

func loadIcons(dist string) (iconIndex, error) {
	ents, err := os.ReadDir(dist)
	if err != nil {
		return iconIndex{}, fmt.Errorf("thesvg dist not readable (%w), run npm ci in web/ or set %s", err, distEnv)
	}
	ix := iconIndex{exact: map[string]bool{}, byKey: map[string][]string{}}
	for _, e := range ents {
		n := e.Name()
		if !strings.HasSuffix(n, ".js") || strings.HasPrefix(n, "index") {
			continue
		}
		slug := strings.TrimSuffix(n, ".js")
		ix.exact[slug] = true
		k := norm(slug)
		ix.byKey[k] = append(ix.byKey[k], slug)
	}
	if len(ix.exact) == 0 {
		return iconIndex{}, errors.New("no icons found in " + dist)
	}
	return ix, nil
}

// resolve returns a verified thesvg slug for the app, or "" when none exists.
func (ix iconIndex) resolve(a App) string {
	if a.Icon != "" && ix.exact[a.Icon] {
		return a.Icon
	}
	cands := []string{strings.ReplaceAll(strings.ToLower(strings.TrimSpace(a.Name)), " ", "-")}
	if a.Icon != "" {
		cands = append(cands, a.Icon)
	}
	for _, c := range cands {
		if ix.exact[c] {
			return c
		}
	}
	for _, k := range keys(a.Name) {
		if hits := ix.byKey[k]; len(hits) > 0 {
			sort.Slice(hits, func(i, j int) bool { return len(hits[i]) < len(hits[j]) })
			return hits[0]
		}
	}
	return ""
}

func classify(apps []App, known registryIndex, icons iconIndex) Report {
	var rep Report
	for _, a := range apps {
		row := Row{App: a, Slug: icons.resolve(a)}
		row.HasLogo = row.Slug != ""
		id, near := known.find(a.Name)
		switch {
		case id != "" && !near:
			row.Status, row.Match = statusCovered, id
			rep.Covered = append(rep.Covered, row)
		case id != "":
			row.Status, row.Match = statusNear, id
			rep.Near = append(rep.Near, row)
		default:
			row.Status = statusMissing
			rep.Missing = append(rep.Missing, row)
		}
	}
	for _, s := range [][]Row{rep.Missing, rep.Near} {
		sort.SliceStable(s, func(i, j int) bool { return s[i].Score > s[j].Score })
	}
	return rep
}

func render(rep Report, top, total int) string {
	var b strings.Builder
	withLogo, noLogo := split(rep.Missing)
	fmt.Fprintf(&b, "# Candidate work list\n\n")
	fmt.Fprintf(&b, "%d list rows: %d covered, %d near duplicates, %d missing (%d with a thesvg logo, %d without).\n\n",
		total, len(rep.Covered), len(rep.Near), len(rep.Missing), len(withLogo), len(noLogo))
	b.WriteString("## Missing apps with a logo (ranked)\n\n")
	b.WriteString("| # | App | Score | Category | Icon slug | Repo |\n| --- | --- | --- | --- | --- | --- |\n")
	for i, r := range clip(withLogo, top) {
		fmt.Fprintf(&b, "| %d | %s | %.0f | %s | %s | %s |\n", i+1, cell(r.Name), r.Score, cell(r.Category), r.Slug, cell(r.Repo))
	}
	b.WriteString("\n## Missing apps without a thesvg logo (needs a thesvg PR)\n\n")
	b.WriteString("| # | App | Score | Category | Repo |\n| --- | --- | --- | --- | --- |\n")
	for i, r := range clip(noLogo, top) {
		fmt.Fprintf(&b, "| %d | %s | %.0f | %s | %s |\n", i+1, cell(r.Name), r.Score, cell(r.Category), cell(r.Repo))
	}
	b.WriteString("\n## Near duplicates (check before adding)\n\n")
	b.WriteString("| App | Close to registry id |\n| --- | --- |\n")
	for _, r := range clip(rep.Near, top) {
		fmt.Fprintf(&b, "| %s | %s |\n", cell(r.Name), r.Match)
	}
	return b.String()
}

func split(rows []Row) (with, without []Row) {
	for _, r := range rows {
		if r.HasLogo {
			with = append(with, r)
		} else {
			without = append(without, r)
		}
	}
	return with, without
}

func clip(rows []Row, n int) []Row {
	if n > 0 && len(rows) > n {
		return rows[:n]
	}
	return rows
}

func cell(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "|", "/"), "\n", " ")
}
