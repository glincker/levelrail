package statuspage

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"html/template"
	"strings"
	"time"
)

// FeedPath and SummaryPath are the public asset paths, relative to the page.
const (
	PagePath    = "/public/status"
	SummaryPath = "/public/status.json"
	FeedPath    = "/public/status.rss"
)

type pageData struct {
	View        View
	Incidents   []incidentHTML
	Maintenance []incidentHTML
	SummaryPath string
	FeedPath    string
}

type incidentHTML struct {
	IncidentView
	Updates []updateHTML
}

type updateHTML struct {
	Status string
	At     string
	Body   template.HTML
}

var pageFuncs = template.FuncMap{
	"label": statusLabel,
	"pct": func(p *float64) string {
		if p == nil {
			return "No data"
		}
		return fmt.Sprintf("%.2f%%", *p)
	},
	"day": func(d DayView) string {
		if d.Uptime == nil {
			return d.Date + ": no data"
		}
		return fmt.Sprintf("%s: %.2f%% (%s)", d.Date, *d.Uptime, statusLabel(d.Status))
	},
	"stamp": func(t time.Time) string { return t.UTC().Format("2006-01-02 15:04 UTC") },
}

func statusLabel(s string) string {
	switch s {
	case Operational:
		return "Operational"
	case Degraded:
		return "Degraded"
	case Outage:
		return "Outage"
	case Maintenance:
		return "Maintenance"
	case NoData:
		return "No data"
	case StatusInvestigating:
		return "Investigating"
	case StatusIdentified:
		return "Identified"
	case StatusMonitoring:
		return "Monitoring"
	case StatusResolved:
		return "Resolved"
	case StatusScheduled:
		return "Scheduled"
	case StatusInProgress:
		return "In progress"
	case StatusCompleted:
		return "Completed"
	default:
		return "Unknown"
	}
}

var pageTemplate = template.Must(template.New("page").Funcs(pageFuncs).Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>{{.View.Title}}</title>
<link rel="alternate" type="application/rss+xml" title="{{.View.Title}}" href="{{.FeedPath}}">
<style>
:root{--bg:#f7f8fa;--fg:#14181f;--muted:#5b6472;--card:#fff;--line:#dfe3ea;--ok:#1a7f4b;--warn:#a15c00;--bad:#b42318;--maint:#3b5bdb;--none:#c9ced8}
@media (prefers-color-scheme:dark){:root{--bg:#0e1116;--fg:#e8ebf0;--muted:#98a2b3;--card:#171b22;--line:#2a303b;--ok:#3ecf8e;--warn:#f5b849;--bad:#ff6b5e;--maint:#8ea2ff;--none:#3a414d}}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--fg);font:16px/1.5 system-ui,-apple-system,"Segoe UI",sans-serif}
main{max-width:760px;margin:0 auto;padding:32px 16px 64px}
h1{font-size:1.6rem;margin:0 0 4px}
h2{font-size:1.1rem;margin:32px 0 12px}
p.lead{color:var(--muted);margin:0 0 20px}
.banner{border:1px solid var(--line);border-left:6px solid var(--ok);background:var(--card);border-radius:8px;padding:14px 16px;font-weight:600}
.banner.degraded{border-left-color:var(--warn)}.banner.outage{border-left-color:var(--bad)}.banner.maintenance{border-left-color:var(--maint)}.banner.unknown{border-left-color:var(--none)}
.card{border:1px solid var(--line);background:var(--card);border-radius:8px;padding:14px 16px;margin-bottom:12px}
.row{display:flex;justify-content:space-between;gap:12px;align-items:baseline;flex-wrap:wrap}
.name{font-weight:600}
.badge{font-size:.85rem;font-weight:600}
.badge.operational{color:var(--ok)}.badge.degraded{color:var(--warn)}.badge.outage{color:var(--bad)}.badge.maintenance{color:var(--maint)}.badge.unknown,.badge.nodata{color:var(--muted)}
.bars{display:flex;gap:2px;margin:10px 0 4px;height:28px}
.bar{flex:1;min-width:2px;border-radius:2px;background:var(--none)}
.bar.operational{background:var(--ok)}.bar.degraded{background:var(--warn)}.bar.outage{background:var(--bad)}
.legend{display:flex;justify-content:space-between;color:var(--muted);font-size:.8rem}
.meta{color:var(--muted);font-size:.85rem}
.update{border-top:1px solid var(--line);margin-top:10px;padding-top:8px}
.update p{margin:4px 0}
a{color:var(--maint)}
footer{margin-top:40px;color:var(--muted);font-size:.85rem}
</style>
</head>
<body>
<main>
<h1>{{.View.Title}}</h1>
{{if .View.Description}}<p class="lead">{{.View.Description}}</p>{{end}}
<div class="banner {{.View.Status}}" role="status">{{.View.StatusText}}</div>

{{if .Maintenance}}<h2>Maintenance</h2>
{{range .Maintenance}}<article class="card"><div class="row"><span class="name">{{.Title}}</span><span class="badge maintenance">{{label .Status}}</span></div>
<div class="meta">{{stamp .StartsAt}}{{if .EndsAt}} to {{stamp .EndsAt.UTC}}{{end}}{{if .Components}} &middot; {{range $i, $c := .Components}}{{if $i}}, {{end}}{{$c}}{{end}}{{end}}</div>
{{range .Updates}}<div class="update"><div class="meta">{{label .Status}} &middot; {{.At}}</div>{{.Body}}</div>{{end}}</article>{{end}}{{end}}

{{if .Incidents}}<h2>Incidents</h2>
{{range .Incidents}}<article class="card"><div class="row"><span class="name">{{.Title}}</span><span class="badge {{if eq .Status "resolved"}}operational{{else}}outage{{end}}">{{label .Status}}</span></div>
<div class="meta">{{stamp .StartsAt}}{{if .Components}} &middot; {{range $i, $c := .Components}}{{if $i}}, {{end}}{{$c}}{{end}}{{end}}</div>
{{range .Updates}}<div class="update"><div class="meta">{{label .Status}} &middot; {{.At}}</div>{{.Body}}</div>{{end}}</article>{{end}}{{end}}

<h2>Components</h2>
{{range .View.Components}}<section class="card" aria-label="{{.Name}}">
<div class="row"><span class="name">{{.Name}}</span><span class="badge {{.Status}}">{{label .Status}}</span></div>
<div class="bars" role="img" aria-label="{{.Name}} uptime over the last 90 days: {{pct .Uptime90}}">{{range .Days}}<span class="bar {{.Status}}" title="{{day .}}"></span>{{end}}</div>
<div class="legend"><span>90 days ago</span><span>{{pct .Uptime90}} uptime</span><span>Today</span></div>
</section>{{else}}<p class="meta">No components are listed yet.</p>{{end}}

<footer>Updated {{stamp .View.GeneratedAt}} &middot; <a href="{{.FeedPath}}">RSS feed</a> &middot; <a href="{{.SummaryPath}}">JSON</a></footer>
</main>
</body>
</html>
`))

// RenderHTML renders the public page.
func RenderHTML(v View) ([]byte, error) {
	data := pageData{View: v, SummaryPath: SummaryPath, FeedPath: FeedPath,
		Incidents: toHTML(v.Incidents), Maintenance: toHTML(v.Maintenance)}
	var buf bytes.Buffer
	if err := pageTemplate.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("statuspage: render html: %w", err)
	}
	return buf.Bytes(), nil
}

func toHTML(in []IncidentView) []incidentHTML {
	out := make([]incidentHTML, 0, len(in))
	for _, i := range in {
		ups := make([]updateHTML, 0, len(i.Updates))
		for j := len(i.Updates) - 1; j >= 0; j-- {
			u := i.Updates[j]
			ups = append(ups, updateHTML{Status: u.Status, At: u.At.UTC().Format("2006-01-02 15:04 UTC"), Body: RenderMarkdownLite(u.Body)})
		}
		out = append(out, incidentHTML{IncidentView: i, Updates: ups})
	}
	return out
}

type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Items       []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string  `xml:"title"`
	GUID        rssGUID `xml:"guid"`
	PubDate     string  `xml:"pubDate"`
	Description string  `xml:"description"`
}

type rssGUID struct {
	IsPermaLink bool   `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}

// RenderRSS renders incidents and maintenance as an RSS 2.0 feed with
// plain-text descriptions. baseURL is the page's own origin, e.g. "https://status.example.com".
func RenderRSS(v View, baseURL string) ([]byte, error) {
	feed := rssFeed{Version: "2.0", Channel: rssChannel{Title: v.Title, Link: baseURL + PagePath, Description: v.StatusText}}
	for _, group := range [][]IncidentView{v.Incidents, v.Maintenance} {
		for _, i := range group {
			feed.Channel.Items = append(feed.Channel.Items, rssItem{
				Title:       fmt.Sprintf("[%s] %s", statusLabel(i.Status), i.Title),
				GUID:        rssGUID{Value: i.ID + ":" + latestStamp(i)},
				PubDate:     latestTime(i).UTC().Format(time.RFC1123Z),
				Description: plainDescription(i),
			})
		}
	}
	out, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("statuspage: render rss: %w", err)
	}
	return append([]byte(xml.Header), out...), nil
}

func latestTime(i IncidentView) time.Time {
	if n := len(i.Updates); n > 0 {
		return i.Updates[n-1].At
	}
	return i.StartsAt
}

func latestStamp(i IncidentView) string { return latestTime(i).UTC().Format(time.RFC3339) }

func plainDescription(i IncidentView) string {
	var parts []string
	if len(i.Components) > 0 {
		parts = append(parts, "Affected: "+strings.Join(i.Components, ", "))
	}
	for j := len(i.Updates) - 1; j >= 0; j-- {
		parts = append(parts, fmt.Sprintf("%s (%s): %s", statusLabel(i.Updates[j].Status), i.Updates[j].At.UTC().Format("2006-01-02 15:04 UTC"), i.Updates[j].Body))
	}
	return strings.Join(parts, "\n")
}
