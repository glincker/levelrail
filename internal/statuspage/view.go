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
	// KindMonitor follows an uptime monitor by id (ADR 054).
	KindMonitor = "monitor"
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
	Accent      string          `json:"accent,omitempty"`
	LogoURL     string          `json:"logo_url,omitempty"`
	About       string          `json:"about,omitempty"`
	Website     string          `json:"website,omitempty"`
	NoIndex     bool            `json:"noindex,omitempty"`
	PoweredBy   bool            `json:"powered_by"`
	Components  []ComponentView `json:"components"`
	Incidents   []IncidentView  `json:"incidents"`
	Maintenance []IncidentView  `json:"maintenance"`
}

// ComponentView is one component as the public sees it.
type ComponentView struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Group       string    `json:"group,omitempty"`
	Status      string    `json:"status"`
	Uptime90    *float64  `json:"uptime_90d"`
	Days        []DayView `json:"days"`
}

// DayView is one uptime bar.
type DayView struct {
	Date   string   `json:"date"`
	Status string   `json:"status"`
	Uptime *float64 `json:"uptime,omitempty"`
	// Level scales the bar colour 1..4 by the day's weighted downtime share.
	Level int `json:"level,omitempty"`
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
	// Ongoing marks maintenance with no end time yet.
	Ongoing bool `json:"ongoing,omitempty"`
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
		if s != Unknown && s != NoData {
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

// downShare is the weighted downtime share of a set of probe counts:
// failed probes count fully, degraded ones by the configured weight.
func downShare(ok, degraded, down int, t Tuning) (float64, bool) {
	total := ok + degraded + down
	if total == 0 {
		return 0, false
	}
	return (float64(down) + t.DegradedWeight*float64(degraded)) / float64(total), true
}

// uptimeFraction is the time-weighted availability of a set of probe counts.
func uptimeFraction(ok, degraded, down int, t Tuning) (float64, bool) {
	share, has := downShare(ok, degraded, down, t)
	return 1 - share, has
}

// dayLevel maps a downtime share to a 0..4 colour intensity so one failed
// probe in a day shows as a faint mark instead of a solid bar.
func dayLevel(share float64, t Tuning) int {
	if share <= 0 {
		return 0
	}
	l := int(share/t.DayOutageShare*4) + 1
	if l > 4 {
		return 4
	}
	return l
}

func dayStatus(d store.StatusDaily, t Tuning) (string, int) {
	share, has := downShare(d.OK, d.Degraded, d.Down, t)
	switch {
	case !has:
		return NoData, 0
	case share >= t.DayOutageShare:
		return Outage, 4
	case d.Down > 0 || d.Degraded > 0:
		return Degraded, dayLevel(share, t)
	default:
		return Operational, 0
	}
}

// buildBars returns UptimeDays bars ending at today (UTC) plus the
// overall uptime across days that have data.
func buildBars(daily []store.StatusDaily, today time.Time) ([]DayView, *float64) {
	t := TuningFromEnv()
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
			view.Status, view.Level = dayStatus(d, t)
			if u, has := uptimeFraction(d.OK, d.Degraded, d.Down, t); has {
				pct := roundPct(u)
				view.Uptime = &pct
			}
		}
		days = append(days, view)
	}
	total, has := uptimeFraction(ok, deg, down, t)
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

// NewIncidentView is the one public serializer for an incident. Webhooks use it
// so they carry exactly the fields the public page does. Pass public updates only.
func NewIncidentView(i store.StatusIncident, componentNames map[string]string, updates []store.StatusIncidentUpdate) IncidentView {
	views := make([]UpdateView, 0, len(updates))
	for _, u := range updates {
		views = append(views, UpdateView{Status: u.Status, Body: u.Body, At: u.CreatedAt.UTC()})
	}
	return incidentView(i, componentNames, views)
}
