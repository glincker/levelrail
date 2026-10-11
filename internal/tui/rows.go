package tui

import (
	"fmt"
	"strconv"
	"strings"
)

// rows returns the current list view's rows with the filter applied.
func (m Model) rows() []Row {
	_, all := m.allRows(m.view)
	if m.filter == "" {
		return all
	}
	needle := strings.ToLower(m.filter)
	var out []Row
	for _, r := range all {
		var b strings.Builder
		for _, c := range r.Cells {
			b.WriteString(c.Text)
			b.WriteByte(' ')
		}
		if strings.Contains(strings.ToLower(b.String()), needle) {
			out = append(out, r)
		}
	}
	return out
}

func (m Model) columns(v viewID) []string {
	cols, _ := m.allRows(v)
	return cols
}

func (m Model) allRows(v viewID) ([]string, []Row) {
	switch v {
	case viewApps:
		return m.appTable()
	case viewDatabases:
		return m.databaseTable()
	case viewDeploys:
		return m.deployTable("")
	case viewNodes:
		return m.nodeTable()
	case viewDomains:
		return m.domainTable()
	case viewAlerts:
		return m.alertTable()
	default:
		return m.backupTable("")
	}
}

func (m Model) appTable() ([]string, []Row) {
	cols := []string{"NAME", "STATUS", "REPLICAS", "IMAGE", "NODE", "DOMAINS"}
	rows := make([]Row, 0, len(m.apps))
	for _, a := range m.apps {
		rows = append(rows, Row{Key: a.Name, Cells: []Cell{
			plain(a.Name), toned(a.Status, a.Tone), plain(replicas(a.Replicas)),
			plain(a.Image), plain(orDash(a.Node)), plain(domainSummary(a.Domains)),
		}})
	}
	return cols, rows
}

func replicas(n int) string {
	if n <= 0 {
		return "1"
	}
	return strconv.Itoa(n)
}

func domainSummary(d []string) string {
	switch len(d) {
	case 0:
		return "-"
	case 1:
		return d[0]
	}
	return fmt.Sprintf("%s +%d", d[0], len(d)-1)
}

func (m Model) databaseTable() ([]string, []Row) {
	cols := []string{"NAME", "ENGINE", "VERSION", "NODE", "STATE", "BACKUP"}
	rows := make([]Row, 0, len(m.databases))
	for _, d := range m.databases {
		state := toned("active", ToneSuccess)
		if d.Stopped {
			state = toned("suspended", ToneNeutral)
		}
		backup := orDash(d.Schedule)
		if d.Schedule != "" && d.Retain > 0 {
			backup = fmt.Sprintf("%s keep %d", d.Schedule, d.Retain)
		}
		rows = append(rows, Row{Key: d.Name, Cells: []Cell{
			plain(d.Name), plain(d.Engine), plain(d.Version), plain(orDash(d.Node)), state, plain(backup),
		}})
	}
	return cols, rows
}

// deployTable lists deployments, only app's when app is non-empty.
func (m Model) deployTable(app string) ([]string, []Row) {
	cols := []string{"ID", "APP", "STATUS", "TRIGGER", "ENV", "COMMIT", "BRANCH", "AGE", "TOOK"}
	now := m.opts.Now()
	var rows []Row
	for _, d := range m.deploys {
		if app != "" && d.App != app {
			continue
		}
		status := d.Status
		if d.IsLive {
			status += " (live)"
		}
		rows = append(rows, Row{Key: d.ID, Ref: d.App, Cells: []Cell{
			plain(shortID(d.ID, 8)), plain(d.App), toned(status, deployTone(d.Status)),
			plain(orDash(d.Trigger)), plain(orDash(d.Env)), plain(orDash(shortID(d.Commit, 7))),
			plain(orDash(d.Branch)), plain(age(now, d.StartedAt)), plain(durationMS(d.DurationMS)),
		}})
	}
	return cols, rows
}

func (m Model) nodeTable() ([]string, []Row) {
	cols := []string{"NAME", "STATUS", "ADDRESS", "SCHEDULING", "SEEN", "LOCAL"}
	now := m.opts.Now()
	rows := make([]Row, 0, len(m.nodes))
	for _, n := range m.nodes {
		sched := toned("open", ToneSuccess)
		if !n.Schedulable {
			sched = toned("cordoned", ToneWarning)
		}
		local := "no"
		if n.Local {
			local = "yes"
		}
		rows = append(rows, Row{Key: n.Name, Cells: []Cell{
			plain(n.Name), toned(n.Status, nodeTone(n.Status)), plain(orDash(n.Address)),
			sched, plain(age(now, n.LastSeen)), plain(local),
		}})
	}
	return cols, rows
}

func (m Model) domainTable() ([]string, []Row) {
	cols := []string{"DOMAIN", "APP", "TLS", "KIND"}
	rows := make([]Row, 0, len(m.domains))
	for _, d := range m.domains {
		rows = append(rows, Row{Key: d.Domain, Ref: d.App, Cells: []Cell{
			plain(d.Domain), plain(orDash(d.App)), plain(orDash(d.TLS)), plain(orDash(d.Kind)),
		}})
	}
	return cols, rows
}

func (m Model) alertTable() ([]string, []Row) {
	cols := []string{"SEVERITY", "SUBJECT", "TITLE", "DETAIL"}
	rows := make([]Row, 0, len(m.attention))
	for i, a := range m.attention {
		ref := ""
		if _, ok := m.findApp(a.Subject); ok {
			ref = a.Subject
		}
		rows = append(rows, Row{Key: strconv.Itoa(i) + ":" + a.Kind + ":" + a.Subject, Ref: ref, Cells: []Cell{
			toned(a.Severity, severityTone(a.Severity)), plain(orDash(a.Subject)), plain(a.Title), plain(a.Detail),
		}})
	}
	return cols, rows
}

// backupTable lists backups, only name's when name is non-empty.
func (m Model) backupTable(name string) ([]string, []Row) {
	cols := []string{"RESOURCE", "KIND", "STATUS", "SIZE", "AGE"}
	now := m.opts.Now()
	var rows []Row
	for i, b := range m.backups {
		if name != "" && b.Resource != name {
			continue
		}
		rows = append(rows, Row{Key: strconv.Itoa(i) + ":" + b.Resource, Cells: []Cell{
			plain(b.Resource), plain(orDash(b.Kind)), toned(b.Status, deployTone(b.Status)),
			plain(humanBytes(b.SizeBytes)), plain(age(now, b.StartedAt)),
		}})
	}
	return cols, rows
}
