package statuspage

import (
	"encoding/xml"
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

type atomFeed struct {
	XMLName xml.Name    `xml:"http://www.w3.org/2005/Atom feed"`
	Title   string      `xml:"title"`
	ID      string      `xml:"id"`
	Updated string      `xml:"updated"`
	Links   []atomLink  `xml:"link"`
	Author  atomAuthor  `xml:"author"`
	Entries []atomEntry `xml:"entry"`
}

type atomLink struct {
	Rel  string `xml:"rel,attr,omitempty"`
	Type string `xml:"type,attr,omitempty"`
	Href string `xml:"href,attr"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

type atomEntry struct {
	Title   string      `xml:"title"`
	ID      string      `xml:"id"`
	Updated string      `xml:"updated"`
	Link    atomLink    `xml:"link"`
	Content atomContent `xml:"content"`
}

type atomContent struct {
	Type string `xml:"type,attr"`
	Text string `xml:",chardata"`
}

// RenderAtom renders incidents and maintenance as an Atom 1.0 feed.
// baseURL is the page's own origin, e.g. "https://status.example.com".
func RenderAtom(v View, baseURL string) ([]byte, error) {
	feed := atomFeed{Title: v.Title, ID: baseURL + PagePath, Author: atomAuthor{Name: v.Title},
		Links: []atomLink{{Rel: "self", Type: "application/atom+xml", Href: baseURL + AtomPath}, {Rel: "alternate", Type: "text/html", Href: baseURL + PagePath}}}
	updated := v.GeneratedAt
	for _, group := range [][]IncidentView{v.Incidents, v.Maintenance} {
		for _, i := range group {
			lt := latestTime(i)
			if lt.After(updated) || len(feed.Entries) == 0 && updated.IsZero() {
				updated = lt
			}
			href := baseURL + IncidentDir + url.PathEscape(i.ID)
			feed.Entries = append(feed.Entries, atomEntry{
				Title:   fmt.Sprintf("[%s] %s", statusLabel(i.Status), i.Title),
				ID:      href,
				Updated: lt.UTC().Format(time.RFC3339),
				Link:    atomLink{Rel: "alternate", Type: "text/html", Href: href},
				Content: atomContent{Type: "text", Text: plainDescription(i)},
			})
		}
	}
	feed.Updated = updated.UTC().Format(time.RFC3339)
	out, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("statuspage: render atom: %w", err)
	}
	return append([]byte(xml.Header), out...), nil
}

var badgeColors = map[string]string{
	Operational: "#2e9e5b", Degraded: "#d9922e", Outage: "#d14343",
	Maintenance: "#3b5bdb", Unknown: "#8a929e", NoData: "#8a929e",
}

const badgeLabelMax = 32

// RenderBadge renders a flat two-part SVG badge. Both texts are escaped
// and only fixed colours are used, so no input reaches the markup raw.
func RenderBadge(label, state string) []byte {
	label = truncateRunes(sanitizeBadgeText(label), badgeLabelMax)
	msg := strings.ToLower(statusLabel(state))
	color, ok := badgeColors[state]
	if !ok {
		color = badgeColors[Unknown]
	}
	lw, mw := textWidth(label), textWidth(msg)
	total := lw + mw
	el, em := html.EscapeString(label), html.EscapeString(msg)
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="20" role="img" aria-label="%s: %s">`, total, el, em)
	fmt.Fprintf(&b, `<title>%s: %s</title>`, el, em)
	fmt.Fprintf(&b, `<rect width="%d" height="20" rx="3" fill="#555"/><rect x="%d" width="%d" height="20" rx="3" fill="%s"/>`, lw, lw, mw, color)
	fmt.Fprintf(&b, `<g fill="#fff" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" font-size="11" text-anchor="middle">`)
	fmt.Fprintf(&b, `<text x="%d" y="14">%s</text><text x="%d" y="14">%s</text></g></svg>`, lw/2, el, lw+mw/2, em)
	return []byte(b.String())
}

func textWidth(s string) int { return utf8.RuneCountInString(s)*7 + 12 }

func sanitizeBadgeText(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
