package tui

import (
	"fmt"
	"strings"
)

// View renders header, tabs, body and footer to exactly m.h lines.
func (m Model) View() string {
	if m.w < 40 || m.h < 8 {
		return "terminal too small (need at least 40x8), resize or press q to quit"
	}
	body := m.bodyLines()
	bodyH := m.h - 4
	if len(body) > bodyH {
		body = body[:bodyH]
	}
	for len(body) < bodyH {
		body = append(body, "")
	}
	lines := append([]string{m.headerLine(), m.tabsLine()}, body...)
	lines = append(lines, m.promptLine(), m.hintLine())
	for i, l := range lines {
		lines[i] = truncate(l, m.w)
	}
	return strings.Join(lines, "\n")
}

func (m Model) headerLine() string {
	parts := []string{m.st.bold.Render(orDash(m.opts.Prog))}
	if m.opts.ReadOnly {
		parts = append(parts, m.st.banner.Render("[READ-ONLY]"))
	}
	if m.opts.Profile != "" {
		parts = append(parts, "profile:"+clean(m.opts.Profile))
	}
	if m.opts.Host != "" {
		parts = append(parts, clean(m.opts.Host))
	}
	ver := "server:?"
	if m.server.Version != "" {
		ver = "server:" + clean(m.server.Version)
		if m.server.UpdateAvailable {
			ver += " (update available)"
		}
	}
	parts = append(parts, ver, m.meshText())
	crit, total := m.incidentCounts()
	inc := fmt.Sprintf("incidents:%d attention:%d", crit, total)
	if crit > 0 {
		inc = m.st.tone[ToneDanger].Render(inc)
	}
	parts = append(parts, inc, "every "+m.opts.Refresh.String())
	return strings.Join(parts, "  ")
}

func (m Model) meshText() string {
	if !m.slowLoaded {
		return "mesh:?"
	}
	if !m.mesh.Enabled {
		return "mesh:off"
	}
	return fmt.Sprintf("mesh:%d/%d", m.mesh.Healthy, m.mesh.Peers)
}

func (m Model) incidentCounts() (critical, total int) {
	for _, a := range m.attention {
		total++
		if strings.EqualFold(a.Severity, "critical") {
			critical++
		}
	}
	return critical, total
}

func (m Model) tabsLine() string {
	parts := make([]string, 0, viewCount)
	for v := viewID(0); v < viewCount; v++ {
		label := fmt.Sprintf("%d %s%s", v+1, viewNames[v], m.count(v))
		if v == m.view && m.screen == screenList {
			label = m.st.tabOn.Render(label)
		} else {
			label = m.st.muted.Render(label)
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, "  ")
}

func (m Model) count(v viewID) string {
	n := 0
	switch v {
	case viewApps:
		n = len(m.apps)
	case viewDatabases:
		n = len(m.databases)
	case viewDeploys:
		n = len(m.deploys)
	case viewNodes:
		n = len(m.nodes)
	case viewDomains:
		n = len(m.domains)
	case viewAlerts:
		n = len(m.attention)
	case viewBackups:
		n = len(m.backups)
	}
	return fmt.Sprintf("(%d)", n)
}

func (m Model) promptLine() string {
	switch m.mode {
	case modeFilter:
		return "/" + m.filter + "_"
	case modeCommand:
		return ":" + m.input + "_"
	case modeConfirm:
		if p := m.pending; p != nil {
			return fmt.Sprintf("%s: type %q to confirm (enter runs, esc cancels) > %s_", p.summary, p.app, m.input)
		}
	}
	if m.notice != "" {
		if m.noticeErr {
			return m.st.tone[ToneDanger].Render(m.notice)
		}
		return m.st.tone[ToneSuccess].Render(m.notice)
	}
	if m.filter != "" {
		return fmt.Sprintf("filter: %s (%d shown, esc clears)", m.filter, len(m.rows()))
	}
	return ""
}

func (m Model) hintLine() string {
	var h string
	switch {
	case m.mode == modeHelp:
		h = "any key closes help"
	case m.screen == screenLogs:
		h = "j/k scroll  g/G top/bottom  f follow  c clear  esc back  ? help"
	case m.screen == screenDetail && m.detailKind == LogDatabase:
		h = "l logs  esc back  ? help"
	case m.screen == screenDetail:
		h = "l logs  " + m.mutationHints() + "esc back  ? help"
	default:
		h = "j/k move  enter open  / filter  : command  l logs  " + m.mutationHints() + "? help  q quit"
	}
	return m.st.muted.Render(h)
}

func (m Model) mutationHints() string {
	if m.opts.ReadOnly {
		return ""
	}
	return "r d b x o actions  "
}

func (m Model) bodyLines() []string {
	switch {
	case m.mode == modeHelp:
		return m.helpLines()
	case m.screen == screenLogs:
		return m.logLines()
	case m.screen == screenDetail:
		return m.detailLines()
	}
	return m.listLines()
}

func (m Model) listLines() []string {
	name := strings.ToLower(viewNames[m.view])
	if msg, bad := m.errs[m.sectionOf(m.view)]; bad {
		if len(m.rows()) == 0 {
			return []string{m.st.tone[ToneDanger].Render(fmt.Sprintf("Could not load %s: %s", name, clean(msg))), "Still retrying; check the API URL and token with `" + m.opts.Prog + " auth whoami`."}
		}
	}
	if !m.loadedFor(m.view) {
		return []string{"Loading " + name + "..."}
	}
	rows := m.rows()
	if len(rows) == 0 {
		if m.filter != "" {
			return []string{fmt.Sprintf("No %s match %q. Press esc to clear the filter.", name, m.filter)}
		}
		return []string{m.emptyCopy()}
	}
	return renderTable(m.st, m.columns(m.view), rows, m.w, m.listHeight(), m.cursor, m.offset)
}

func (m Model) loadedFor(v viewID) bool {
	if v == viewApps || v == viewDeploys || v == viewAlerts {
		return m.fastLoaded
	}
	return m.slowLoaded
}

func (m Model) sectionOf(v viewID) string {
	return [viewCount]string{SecApps, SecDatabases, SecDeploys, SecNodes, SecDomains, SecAttention, SecBackups}[v]
}

func (m Model) emptyCopy() string {
	p := m.opts.Prog
	switch m.view {
	case viewApps:
		return "No apps yet. Create one with `" + p + " apps create` or from the dashboard."
	case viewDatabases:
		return "No databases yet. Create one with `" + p + " databases create`."
	case viewDeploys:
		return "No deployments yet. Press 1, pick an app and press d to deploy."
	case viewNodes:
		return "No nodes registered. Enroll one with `" + p + " nodes`."
	case viewDomains:
		return "No domains yet. Add one with `" + p + " domains` or from the dashboard."
	case viewAlerts:
		return "Nothing needs attention."
	default:
		return "No backups recorded yet. Configure a backup target and schedule in the dashboard."
	}
}

func (m Model) logLines() []string {
	l := m.logs
	title := fmt.Sprintf("logs: %s", clean(l.name))
	mode := "following"
	if !l.follow {
		mode = "paused"
	}
	out := []string{m.st.bold.Render(title) + m.st.muted.Render(fmt.Sprintf("  [%s, %d lines]", mode, len(l.lines)))}
	if l.ended != "" {
		out[0] += "  " + m.st.tone[ToneWarning].Render(l.ended+", press l from the list to reopen")
	}
	h := m.h - 5
	top := l.top
	maxTop := len(l.lines) - h
	if maxTop < 0 {
		maxTop = 0
	}
	if l.follow || top > maxTop {
		top = maxTop
	}
	if len(l.lines) == 0 && l.ended == "" {
		return append(out, "Waiting for log output...")
	}
	for i := top; i < len(l.lines) && i < top+h; i++ {
		e := l.lines[i]
		out = append(out, m.st.tone[e.tone].Render(truncate(e.text, m.w)))
	}
	return out
}

func (m Model) helpLines() []string {
	lines := []string{
		m.st.bold.Render("Keys"),
		"  j/k, up/down      move          g/G   top/bottom      ctrl-d/ctrl-u  half page",
		"  1-7, tab          switch view   enter drill into row    esc/q          back",
		"  /                 filter rows   :     command bar       l              logs",
		"  f (in logs)       follow        c     clear logs      ctrl-c         quit",
	}
	if m.opts.ReadOnly {
		lines = append(lines, "", "Read-only mode: restart, deploy, rollback, stop and start are disabled.")
	} else {
		lines = append(lines,
			"  r restart   d deploy   b rollback   x stop   o start   (each asks you to type the app name)",
			"",
			m.st.bold.Render("Commands"),
			"  :deploy [app]  :restart [app]  :rollback [app]  :stop [app]  :start [app]",
			"  :logs [name]   :apps :databases :deploys :nodes :domains :alerts :backups   :q",
		)
	}
	lines = append(lines, "", "Secrets and env values are never shown here; use the dashboard to reveal them.")
	return lines
}
