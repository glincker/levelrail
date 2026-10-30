package statuspage

import (
	"bytes"
	"fmt"
	"html/template"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"sync"

	"github.com/GLINCKER/levelrail/internal/brand"
)

var accentRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type pageData struct {
	View        View
	Heading     string
	Accent      template.CSS
	Incidents   []incidentHTML
	Maintenance []incidentHTML
	Ungrouped   []ComponentView
	Groups      []groupHTML
	Entries     []incidentHTML
	Incident    *incidentHTML
	Tab         string
	Page        int
	Pages       int
	SummaryPath string
	FeedPath    string
	AtomPath    string
	HistoryPath string
	CreditName  string
	CreditURL   string
}

type groupHTML struct {
	Name       string
	Status     string
	Components []ComponentView
}

type incidentHTML struct {
	IncidentView
	Kind    string
	Updates []updateHTML
}

type updateHTML struct {
	Status string
	At     string
	Body   template.HTML
}

func newPageData(v View, heading string) pageData {
	d := pageData{View: v, Heading: heading, SummaryPath: SummaryPath, FeedPath: FeedPath, AtomPath: AtomPath, HistoryPath: HistoryPath}
	d.CreditName, d.CreditURL = credit()
	if accentRe.MatchString(v.Accent) {
		// #nosec G203 -- the value matched a strict hex colour pattern
		d.Accent = template.CSS(v.Accent)
	}
	return d
}

func histURL(tab string, page int) string {
	q := url.Values{}
	if tab != "" && tab != "all" {
		q.Set("tab", tab)
	}
	if page > 1 {
		q.Set("page", strconv.Itoa(page))
	}
	if len(q) == 0 {
		return HistoryPath
	}
	return HistoryPath + "?" + q.Encode()
}

// groupComponents splits components into the ungrouped ones and one
// block per group, each with its worst member status.
func groupComponents(comps []ComponentView) ([]ComponentView, []groupHTML) {
	var ungrouped []ComponentView
	var groups []groupHTML
	idx := map[string]int{}
	for _, c := range comps {
		if c.Group == "" {
			ungrouped = append(ungrouped, c)
			continue
		}
		i, ok := idx[c.Group]
		if !ok {
			i = len(groups)
			idx[c.Group] = i
			groups = append(groups, groupHTML{Name: c.Group})
		}
		groups[i].Components = append(groups[i].Components, c)
	}
	for i := range groups {
		st := make([]string, len(groups[i].Components))
		for j, c := range groups[i].Components {
			st[j] = c.Status
		}
		groups[i].Status = OverallStatus(st)
	}
	return ungrouped, groups
}

const pageHead = `{{define "top"}}<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
{{if .View.NoIndex}}<meta name="robots" content="noindex">
{{end}}<title>{{.Heading}}</title>
<link rel="alternate" type="application/rss+xml" title="{{.View.Title}}" href="{{.FeedPath}}">
<link rel="alternate" type="application/atom+xml" title="{{.View.Title}}" href="{{.AtomPath}}">
<style>
:root{--bg:#f7f8fa;--fg:#14181f;--muted:#5b6472;--card:#fff;--line:#dfe3ea;--ok:#1a7f4b;--warn:#a15c00;--bad:#b42318;--maint:#3b5bdb;--none:#c9ced8;--accent:{{if .Accent}}{{.Accent}}{{else}}#3b5bdb{{end}}}
@media (prefers-color-scheme:dark){:root{--bg:#0e1116;--fg:#e8ebf0;--muted:#98a2b3;--card:#171b22;--line:#2a303b;--ok:#3ecf8e;--warn:#f5b849;--bad:#ff6b5e;--maint:#8ea2ff;--none:#3a414d}}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--fg);font:16px/1.5 system-ui,-apple-system,"Segoe UI",sans-serif}
main{max-width:760px;margin:0 auto;padding:32px 16px 64px}
header.brand{display:flex;align-items:center;gap:12px;border-top:4px solid var(--accent);padding-top:16px;margin-bottom:8px}
header.brand img{max-height:40px;max-width:160px}
h1{font-size:1.6rem;margin:0 0 4px}
h2{font-size:1.1rem;margin:32px 0 12px}
p.lead{color:var(--muted);margin:0 0 20px}
p.about{white-space:pre-line;margin:0 0 20px}
.banner{border:1px solid var(--line);border-left:6px solid var(--ok);background:var(--card);border-radius:8px;padding:14px 16px;font-weight:600}
.banner.degraded{border-left-color:var(--warn)}.banner.outage{border-left-color:var(--bad)}.banner.maintenance{border-left-color:var(--maint)}.banner.unknown,.banner.nodata{border-left-color:var(--none)}
.card{border:1px solid var(--line);background:var(--card);border-radius:8px;padding:14px 16px;margin-bottom:12px}
.row{display:flex;justify-content:space-between;gap:12px;align-items:baseline;flex-wrap:wrap}
.name{font-weight:600}
.badge{font-size:.85rem;font-weight:600}
.badge.operational{color:var(--ok)}.badge.degraded{color:var(--warn)}.badge.outage{color:var(--bad)}.badge.maintenance{color:var(--maint)}.badge.unknown,.badge.nodata{color:var(--muted)}
details.group{margin-bottom:12px}
details.group>summary{cursor:pointer;display:flex;justify-content:space-between;gap:12px;font-weight:600;padding:8px 4px}
details.group>.card{margin-left:8px}
.bars{display:flex;gap:2px;margin:10px 0 4px;height:28px}
.bar{flex:1;min-width:2px;border-radius:2px;background:var(--none)}
.bar.operational{background:var(--ok)}.bar.degraded{background:var(--warn)}.bar.outage{background:var(--bad)}
.bar.lv1{opacity:.4}.bar.lv2{opacity:.6}.bar.lv3{opacity:.8}
.legend{display:flex;justify-content:space-between;color:var(--muted);font-size:.8rem}
.meta{color:var(--muted);font-size:.85rem}
.update{border-top:1px solid var(--line);margin-top:10px;padding-top:8px}
.update p{margin:4px 0}
.tabs{display:flex;gap:16px;margin:16px 0}
.tabs a{text-decoration:none;padding-bottom:4px}
.tabs a.on{border-bottom:2px solid var(--accent);font-weight:600}
.pager{display:flex;justify-content:space-between;margin-top:16px}
a{color:var(--accent)}
footer{margin-top:40px;color:var(--muted);font-size:.85rem}
</style>
</head>
<body>
<main>
<header class="brand">{{if .View.LogoURL}}<img src="{{.View.LogoURL}}" alt="" width="160" height="40">{{end}}<div><h1>{{.View.Title}}</h1></div></header>
{{end}}

{{define "foot"}}<footer>Updated {{stamp .View.GeneratedAt}} &middot; <a href="{{.HistoryPath}}">History</a> &middot; <a href="{{.AtomPath}}">Atom</a> &middot; <a href="{{.FeedPath}}">RSS</a> &middot; <a href="{{.SummaryPath}}">JSON</a>{{if .View.Website}} &middot; <a href="{{.View.Website}}" rel="noopener noreferrer">Website</a>{{end}}{{if and .View.PoweredBy .CreditURL}} &middot; Powered by <a href="{{.CreditURL}}" rel="noopener noreferrer">{{.CreditName}}</a>{{end}}</footer>
</main>
</body>
</html>
{{end}}

{{define "incident"}}<article class="card"><div class="row"><span class="name"><a href="{{incidentHref .ID}}">{{.Title}}</a></span><span class="badge {{if eq .Kind "maintenance"}}maintenance{{else if eq .Status "resolved"}}operational{{else}}outage{{end}}">{{label .Status}}</span></div>
<div class="meta">{{stamp .StartsAt}}{{if .EndsAt}} to {{stamp .EndsAt.UTC}}{{end}}{{if .Ongoing}} &middot; ongoing{{end}}{{if .Components}} &middot; {{range $i, $c := .Components}}{{if $i}}, {{end}}{{$c}}{{end}}{{end}}</div>
{{range .Updates}}<div class="update"><div class="meta">{{label .Status}} &middot; {{.At}}</div>{{.Body}}</div>{{end}}</article>{{end}}

{{define "component"}}<section class="card" aria-label="{{.Name}}">
<div class="row"><span class="name">{{.Name}}</span><span class="badge {{.Status}}">{{label .Status}}</span></div>
{{if .Description}}<div class="meta">{{.Description}}</div>{{end}}
<div class="bars" role="img" aria-label="{{.Name}} uptime over the last 90 days: {{pct .Uptime90}}">{{range .Days}}<span class="bar {{.Status}}{{if .Level}} lv{{.Level}}{{end}}" title="{{day .}}"></span>{{end}}</div>
<div class="legend"><span>90 days ago</span><span>{{pct .Uptime90}} uptime</span><span>Today</span></div>
</section>{{end}}
`

const pageMain = `{{define "page"}}{{template "top" .}}
{{if .View.Description}}<p class="lead">{{.View.Description}}</p>{{end}}
{{if .View.About}}<p class="about">{{.View.About}}</p>{{end}}
<div class="banner {{.View.Status}}" role="status">{{.View.StatusText}}</div>

{{if .Maintenance}}<h2>Maintenance</h2>
{{range .Maintenance}}{{template "incident" .}}{{end}}{{end}}

{{if .Incidents}}<h2>Incidents</h2>
{{range .Incidents}}{{template "incident" .}}{{end}}{{end}}

<h2>Components</h2>
{{range .Ungrouped}}{{template "component" .}}{{end}}
{{range .Groups}}<details class="group" open><summary><span>{{.Name}}</span><span class="badge {{.Status}}">{{label .Status}}</span></summary>
{{range .Components}}{{template "component" .}}{{end}}</details>{{end}}
{{if not .View.Components}}<p class="meta">No components are listed yet.</p>{{end}}

{{template "foot" .}}{{end}}

{{define "history"}}{{template "top" .}}
<h2>History</h2>
<nav class="tabs" aria-label="History filter"><a href="{{histURL "all" 1}}"{{if eq .Tab "all"}} class="on" aria-current="page"{{end}}>All</a><a href="{{histURL "incidents" 1}}"{{if eq .Tab "incidents"}} class="on" aria-current="page"{{end}}>Incidents</a><a href="{{histURL "maintenance" 1}}"{{if eq .Tab "maintenance"}} class="on" aria-current="page"{{end}}>Maintenance</a></nav>
{{range .Entries}}{{template "incident" .}}{{else}}<p class="meta">Nothing to show in this period.</p>{{end}}
{{if gt .Pages 1}}<nav class="pager" aria-label="Pages">{{if gt .Page 1}}<a href="{{histURL .Tab (dec .Page)}}" rel="prev">Newer</a>{{else}}<span></span>{{end}}<span class="meta">Page {{.Page}} of {{.Pages}}</span>{{if lt .Page .Pages}}<a href="{{histURL .Tab (inc .Page)}}" rel="next">Older</a>{{else}}<span></span>{{end}}</nav>{{end}}
{{template "foot" .}}{{end}}

{{define "incidentpage"}}{{template "top" .}}
{{with .Incident}}{{template "incident" .}}{{end}}
<p><a href="{{.HistoryPath}}">All incidents</a></p>
{{template "foot" .}}{{end}}
`

var pageTemplate = template.Must(template.New("page").Funcs(pageFuncs).Funcs(template.FuncMap{
	"incidentHref": func(id string) string { return IncidentDir + url.PathEscape(id) },
	"inc":          func(n int) int { return n + 1 },
	"dec":          func(n int) int { return n - 1 },
}).Parse(pageHead + pageMain))

func execPage(name string, d pageData) ([]byte, error) {
	var buf bytes.Buffer
	if err := pageTemplate.ExecuteTemplate(&buf, name, d); err != nil {
		return nil, fmt.Errorf("statuspage: render %s: %w", name, err)
	}
	return buf.Bytes(), nil
}

// RenderHTML renders the public page.
func RenderHTML(v View) ([]byte, error) {
	d := newPageData(v, v.Title)
	d.Incidents, d.Maintenance = toHTML(v.Incidents, IncidentKind), toHTML(v.Maintenance, MaintenanceKind)
	d.Ungrouped, d.Groups = groupComponents(v.Components)
	return execPage("page", d)
}

// RenderHistory renders one page of the incident and maintenance archive.
// tab is "all", "incidents" or "maintenance"; page counts from 1.
func RenderHistory(v View, tab string, page, size int) ([]byte, error) {
	if size <= 0 {
		size = 10
	}
	var all []incidentHTML
	if tab != "maintenance" {
		all = append(all, toHTML(v.Incidents, IncidentKind)...)
	}
	if tab != "incidents" {
		all = append(all, toHTML(v.Maintenance, MaintenanceKind)...)
	}
	if tab != "incidents" && tab != "maintenance" {
		tab = "all"
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].StartsAt.After(all[j].StartsAt) })
	pages := (len(all) + size - 1) / size
	if pages < 1 {
		pages = 1
	}
	if page < 1 {
		page = 1
	}
	if page > pages {
		page = pages
	}
	lo, hi := (page-1)*size, page*size
	if hi > len(all) {
		hi = len(all)
	}
	d := newPageData(v, "History - "+v.Title)
	d.Entries, d.Tab, d.Page, d.Pages = all[lo:hi], tab, page, pages
	for i := range d.Entries {
		d.Entries[i].Updates = nil
	}
	return execPage("history", d)
}

// RenderIncident renders the permalink page for one incident or
// maintenance entry. ok is false when the ID is unknown.
func RenderIncident(v View, id string) (body []byte, ok bool, err error) {
	var found *incidentHTML
	for _, list := range []struct {
		in   []IncidentView
		kind string
	}{{v.Incidents, IncidentKind}, {v.Maintenance, MaintenanceKind}} {
		for _, i := range list.in {
			if i.ID == id {
				h := toHTML([]IncidentView{i}, list.kind)[0]
				found = &h
			}
		}
	}
	if found == nil {
		return nil, false, nil
	}
	d := newPageData(v, found.Title+" - "+v.Title)
	d.Incident = found
	body, err = execPage("incidentpage", d)
	return body, true, err
}

func toHTML(in []IncidentView, kind string) []incidentHTML {
	out := make([]incidentHTML, 0, len(in))
	for _, i := range in {
		ups := make([]updateHTML, 0, len(i.Updates))
		for j := len(i.Updates) - 1; j >= 0; j-- {
			u := i.Updates[j]
			ups = append(ups, updateHTML{Status: u.Status, At: u.At.UTC().Format("2006-01-02 15:04 UTC"), Body: RenderMarkdownLite(u.Body)})
		}
		out = append(out, incidentHTML{IncidentView: i, Kind: kind, Updates: ups})
	}
	return out
}

var (
	creditOnce sync.Once
	creditName string
	creditURL  string
)

// credit returns the embedded brand name and repo link for the footer.
func credit() (string, string) {
	creditOnce.Do(func() {
		if b, err := brand.LoadEmbedded(); err == nil {
			creditName, creditURL = b.Name, b.RepoURL
		}
	})
	return creditName, creditURL
}
