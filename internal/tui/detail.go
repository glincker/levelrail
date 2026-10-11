package tui

import (
	"fmt"
	"strings"
)

const detailRecentRows = 8

func (m Model) detailLines() []string {
	if m.detailKind == LogDatabase {
		return m.databaseDetail()
	}
	return m.appDetail()
}

func kv(m Model, k, v string) string {
	return m.st.muted.Render(pad(k, 10)) + clean(v)
}

func (m Model) appDetail() []string {
	a, ok := m.findApp(m.detailName)
	if !ok {
		return []string{fmt.Sprintf("App %q was not found (deleted, or not visible to this profile). Press esc to go back.", clean(m.detailName))}
	}
	out := []string{
		m.st.bold.Render(clean(a.Name)) + "  " + m.st.tone[a.Tone].Render(clean(a.Status)),
		kv(m, "image", orDash(a.Image)),
		kv(m, "replicas", replicas(a.Replicas)+"   strategy: "+orDash(a.Strategy)+"   node: "+orDash(a.Node)),
		kv(m, "domains", strings.Join(nonEmpty(a.Domains), ", ")),
		"",
		m.st.bold.Render("Recent deploys"),
	}
	cols, rows := m.deployTable(a.Name)
	return append(out, m.subTable(cols, rows, "No deploys recorded for this app yet.")...)
}

func (m Model) databaseDetail() []string {
	var d DBRow
	found := false
	for _, x := range m.databases {
		if x.Name == m.detailName {
			d, found = x, true
		}
	}
	if !found {
		return []string{fmt.Sprintf("Database %q was not found. Press esc to go back.", clean(m.detailName))}
	}
	state := "active"
	if d.Stopped {
		state = "suspended"
	}
	access := "private"
	if d.Public {
		access = "publicly reachable"
	}
	out := []string{
		m.st.bold.Render(clean(d.Name)) + "  " + clean(d.Engine) + " " + clean(d.Version),
		kv(m, "state", state+"   access: "+access+"   node: "+orDash(d.Node)),
		kv(m, "backups", orDash(d.Schedule)),
		"",
		m.st.bold.Render("Recent backups"),
	}
	cols, rows := m.backupTable(d.Name)
	return append(out, m.subTable(cols, rows, "No backups recorded for this database yet.")...)
}

func (m Model) subTable(cols []string, rows []Row, empty string) []string {
	if len(rows) == 0 {
		return []string{empty}
	}
	if len(rows) > detailRecentRows {
		rows = rows[:detailRecentRows]
	}
	return renderTable(m.st, cols, rows, m.w, detailRecentRows, -1, 0)
}

func nonEmpty(in []string) []string {
	if len(in) == 0 {
		return []string{"-"}
	}
	return in
}
