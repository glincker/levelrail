// Package statuspage builds the opt-in public status page: component
// probing, 90-day uptime bars, operator-authored incidents and the
// whitelisted public view. Nothing beyond View is ever published.
package statuspage

import (
	"html"
	"html/template"
	"regexp"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Component and overall statuses.
const (
	Operational = "operational"
	Degraded    = "degraded"
	Outage      = "outage"
	Maintenance = "maintenance"
	Unknown     = "unknown"
	NoData      = "nodata"
)

// Component kinds.
const (
	KindApp    = "app"
	KindDomain = "domain"
	KindCheck  = "check"
)

// Incident kinds, statuses and impacts.
const (
	IncidentKind    = "incident"
	MaintenanceKind = "maintenance"

	StatusInvestigating = "investigating"
	StatusIdentified    = "identified"
	StatusMonitoring    = "monitoring"
	StatusResolved      = "resolved"
	StatusScheduled     = "scheduled"
	StatusInProgress    = "in_progress"
	StatusCompleted     = "completed"

	ImpactNone     = "none"
	ImpactMinor    = "minor"
	ImpactMajor    = "major"
	ImpactCritical = "critical"
)

// UptimeDays is the number of daily bars on the page.
const UptimeDays = 90

// recentWindow is how long resolved incidents and completed maintenance stay listed.
const recentWindow = 14 * 24 * time.Hour

// View is the entire public payload. Every field is deliberate: a
// privacy test fails when one is added without updating its whitelist.
type View struct {
	Title       string          `json:"title"`
	Description string          `json:"description,omitempty"`
	Status      string          `json:"status"`
	StatusText  string          `json:"status_text"`
	GeneratedAt time.Time       `json:"generated_at"`
	Components  []ComponentView `json:"components"`
	Incidents   []IncidentView  `json:"incidents"`
	Maintenance []IncidentView  `json:"maintenance"`
}

// ComponentView is one component as the public sees it.
type ComponentView struct {
	Name     string    `json:"name"`
	Status   string    `json:"status"`
	Uptime90 *float64  `json:"uptime_90d"`
	Days     []DayView `json:"days"`
}

// DayView is one uptime bar.
type DayView struct {
	Date   string   `json:"date"`
	Status string   `json:"status"`
	Uptime *float64 `json:"uptime,omitempty"`
}

// IncidentView is an incident or maintenance announcement.
type IncidentView struct {
	ID         string       `json:"id"`
	Title      string       `json:"title"`
	Status     string       `json:"status"`
	Impact     string       `json:"impact"`
	StartsAt   time.Time    `json:"starts_at"`
	EndsAt     *time.Time   `json:"ends_at,omitempty"`
	ResolvedAt *time.Time   `json:"resolved_at,omitempty"`
	Components []string     `json:"components,omitempty"`
	Updates    []UpdateView `json:"updates"`
}

// UpdateView is one timeline entry.
type UpdateView struct {
	Status string    `json:"status"`
	Body   string    `json:"body"`
	At     time.Time `json:"at"`
}

func severity(s string) int {
	switch s {
	case Outage:
		return 3
	case Degraded:
		return 2
	case Maintenance:
		return 1
	default:
		return 0
	}
}

// OverallStatus folds component statuses into one headline status.
func OverallStatus(statuses []string) string {
	worst, anyKnown := Operational, false
	for _, s := range statuses {
		if s != Unknown {
			anyKnown = true
		}
		if severity(s) > severity(worst) {
			worst = s
		}
	}
	if len(statuses) > 0 && !anyKnown {
		return Unknown
	}
	return worst
}

// StatusText is the human headline for an overall status.
func StatusText(s string) string {
	switch s {
	case Operational:
		return "All systems operational"
	case Degraded:
		return "Degraded performance"
	case Outage:
		return "Service disruption"
	case Maintenance:
		return "Scheduled maintenance in progress"
	default:
		return "Status unavailable"
	}
}

func impactStatus(impact string) string {
	switch impact {
	case ImpactMinor:
		return Degraded
	case ImpactMajor, ImpactCritical:
		return Outage
	default:
		return Operational
	}
}

// dayOutageShare is the share of failed probes at which a day's bar turns red.
const dayOutageShare = 0.05

func uptimeFraction(ok, degraded, down int) (float64, bool) {
	total := ok + degraded + down
	if total == 0 {
		return 0, false
	}
	return 1 - float64(down)/float64(total), true
}

func dayStatus(d store.StatusDaily) string {
	total := d.OK + d.Degraded + d.Down
	switch {
	case total == 0:
		return NoData
	case float64(d.Down)/float64(total) >= dayOutageShare:
		return Outage
	case d.Down > 0 || d.Degraded > 0:
		return Degraded
	default:
		return Operational
	}
}

// buildBars returns UptimeDays bars ending at today (UTC) plus the
// overall uptime across days that have data.
func buildBars(daily []store.StatusDaily, today time.Time) ([]DayView, *float64) {
	byDay := make(map[string]store.StatusDaily, len(daily))
	var ok, deg, down int
	for _, d := range daily {
		byDay[d.Day] = d
		ok, deg, down = ok+d.OK, deg+d.Degraded, down+d.Down
	}
	days := make([]DayView, 0, UptimeDays)
	for i := UptimeDays - 1; i >= 0; i-- {
		date := today.AddDate(0, 0, -i).UTC().Format("2006-01-02")
		d, found := byDay[date]
		view := DayView{Date: date, Status: NoData}
		if found {
			view.Status = dayStatus(d)
			if u, has := uptimeFraction(d.OK, d.Degraded, d.Down); has {
				pct := roundPct(u)
				view.Uptime = &pct
			}
		}
		days = append(days, view)
	}
	total, has := uptimeFraction(ok, deg, down)
	if !has {
		return days, nil
	}
	pct := roundPct(total)
	return days, &pct
}

func roundPct(f float64) float64 {
	return float64(int(f*10000+0.5)) / 100
}

var (
	linkPattern = regexp.MustCompile(`\[([^\]\n]+)\]\((https?://[^\s)<>"]+)\)`)
	boldPattern = regexp.MustCompile(`\*\*([^*\n]+)\*\*`)
	codePattern = regexp.MustCompile("`([^`\n]+)`")
)

// RenderMarkdownLite converts operator text to safe HTML. All input is
// escaped first; only paragraphs, line breaks, "- " lists, **bold**,
// `code` and http(s) links are then re-introduced.
func RenderMarkdownLite(src string) template.HTML {
	var out strings.Builder
	inList := false
	closeList := func() {
		if inList {
			out.WriteString("</ul>")
			inList = false
		}
	}
	for _, block := range strings.Split(strings.ReplaceAll(strings.TrimSpace(src), "\r\n", "\n"), "\n\n") {
		var para []string
		flush := func() {
			if len(para) > 0 {
				out.WriteString("<p>" + strings.Join(para, "<br>") + "</p>")
				para = nil
			}
		}
		for _, line := range strings.Split(block, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "- ") {
				flush()
				if !inList {
					out.WriteString("<ul>")
					inList = true
				}
				out.WriteString("<li>" + renderInline(strings.TrimPrefix(trimmed, "- ")) + "</li>")
				continue
			}
			closeList()
			if trimmed != "" {
				para = append(para, renderInline(trimmed))
			}
		}
		flush()
		closeList()
	}
	// #nosec G203 -- every byte of user text passes html.EscapeString before tags are added
	return template.HTML(out.String()) //nolint:gosec // see comment above
}

func renderInline(line string) string {
	s := html.EscapeString(line)
	s = codePattern.ReplaceAllString(s, "<code>$1</code>")
	s = boldPattern.ReplaceAllString(s, "<strong>$1</strong>")
	return linkPattern.ReplaceAllString(s, `<a href="$2" rel="noopener noreferrer nofollow" target="_blank">$1</a>`)
}
