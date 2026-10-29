package statuspage

import (
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
	AtomPath    = "/public/status.atom"
	HistoryPath = "/public/status/history"
	BadgePath   = "/public/status/badge.svg"
	IncidentDir = "/public/status/incidents/"
)

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
	"stamp":   func(t time.Time) string { return t.UTC().Format("2006-01-02 15:04 UTC") },
	"histURL": histURL,
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
