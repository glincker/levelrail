package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const readOnlyNotice = "read-only mode: this action is disabled"

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m.quit()
	}
	switch m.mode {
	case modeFilter:
		return m.keyFilter(msg), nil
	case modeCommand:
		return m.keyCommand(msg)
	case modeConfirm:
		return m.keyConfirm(msg)
	case modeHelp:
		m.mode = modeNormal
		return m, nil
	}
	if m.screen == screenLogs {
		return m.keyLogs(msg)
	}
	return m.keyNormal(msg)
}

func (m Model) quit() (tea.Model, tea.Cmd) {
	m.stopLogs()
	return m, tea.Quit
}

// typed applies one text-entry key to buf; non-text keys leave it as is.
func typed(msg tea.KeyMsg, buf string) string {
	switch msg.Type {
	case tea.KeyRunes:
		return buf + clean(string(msg.Runes))
	case tea.KeySpace:
		return buf + " "
	case tea.KeyBackspace:
		if r := []rune(buf); len(r) > 0 {
			return string(r[:len(r)-1])
		}
		return buf
	}
	return buf
}

func (m Model) keyFilter(msg tea.KeyMsg) Model {
	switch msg.Type {
	case tea.KeyEnter:
		m.mode = modeNormal
	case tea.KeyEsc:
		m.mode, m.filter = modeNormal, ""
	default:
		m.filter = typed(msg, m.filter)
	}
	m.cursor, m.offset = 0, 0
	return m
}

func (m Model) keyCommand(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		line := strings.TrimSpace(m.input)
		m.mode, m.input = modeNormal, ""
		return m.runCommand(line)
	case tea.KeyEsc:
		m.mode, m.input = modeNormal, ""
		return m, nil
	}
	m.input = typed(msg, m.input)
	return m, nil
}

func (m Model) keyConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode, m.input, m.pending = modeNormal, "", nil
		m.notice, m.noticeErr = "cancelled", false
		return m, nil
	case tea.KeyEnter:
		p := m.pending
		if p == nil || strings.TrimSpace(m.input) != p.app {
			m.input = ""
			m.notice, m.noticeErr = "name did not match, nothing was changed", true
			return m, nil
		}
		m.mode, m.input, m.pending = modeNormal, "", nil
		m.notice, m.noticeErr = actionVerbs[p.kind]+" "+p.app+"...", false
		return m, m.runAction(*p)
	}
	m.input = typed(msg, m.input)
	return m, nil
}

func (m Model) keyNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.notice = ""
	k := msg.String()
	if len(k) == 1 && k[0] >= '1' && k[0] <= '7' && m.screen == screenList {
		return m.switchView(viewID(k[0] - '1')), nil
	}
	switch k {
	case "q":
		if m.screen != screenList {
			return m.back(), nil
		}
		return m.quit()
	case "esc":
		return m.escape(), nil
	case "?":
		m.mode = modeHelp
	case "/":
		if m.screen == screenList {
			m.mode, m.filter = modeFilter, ""
		}
	case ":":
		m.mode, m.input = modeCommand, ""
	case "tab":
		return m.switchView((m.view + 1) % viewCount), nil
	case "shift+tab":
		return m.switchView((m.view + viewCount - 1) % viewCount), nil
	case "j", "down":
		return m.move(1), nil
	case "k", "up":
		return m.move(-1), nil
	case "g", "home":
		return m.moveTo(0), nil
	case "G", "end":
		return m.moveTo(1 << 30), nil
	case "ctrl+d", "pgdown":
		return m.move(m.listHeight() / 2), nil
	case "ctrl+u", "pgup":
		return m.move(-m.listHeight() / 2), nil
	case "enter":
		return m.drill(), nil
	case "l":
		return m.openLogs("")
	case "r":
		return m.prepare(actRestart, "")
	case "d":
		return m.prepare(actDeploy, "")
	case "b":
		return m.prepare(actRollback, "")
	case "x":
		return m.prepare(actStop, "")
	case "o":
		return m.prepare(actStart, "")
	}
	return m, nil
}

func (m Model) escape() Model {
	switch {
	case m.screen != screenList:
		return m.back()
	case m.filter != "":
		m.filter, m.cursor, m.offset = "", 0, 0
	}
	return m
}

func (m Model) back() Model {
	m.stopLogs()
	if m.screen == screenLogs && m.logs.from == screenDetail {
		m.screen = screenDetail
		return m
	}
	m.screen = screenList
	return m
}

func (m Model) switchView(v viewID) Model {
	m.stopLogs()
	m.view, m.screen = v, screenList
	m.cursor, m.offset, m.filter = 0, 0, ""
	return m
}

func (m Model) move(delta int) Model { return m.moveTo(m.cursor + delta) }

func (m Model) moveTo(i int) Model {
	if m.screen != screenList {
		return m
	}
	n := len(m.rows())
	if i >= n {
		i = n - 1
	}
	if i < 0 {
		i = 0
	}
	m.cursor = i
	m.fixScroll()
	return m
}

func (m *Model) fixScroll() {
	vis := m.listHeight()
	if vis < 1 {
		vis = 1
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+vis {
		m.offset = m.cursor - vis + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

// listHeight is how many table rows fit under header, tabs and footer.
func (m Model) listHeight() int {
	h := m.h - 4 - 1
	if h < 1 {
		return 1
	}
	return h
}

func (m Model) selectedRow() (Row, bool) {
	rows := m.rows()
	if m.cursor < 0 || m.cursor >= len(rows) {
		return Row{}, false
	}
	return rows[m.cursor], true
}

func (m Model) selectedKey() string {
	if r, ok := m.selectedRow(); ok {
		return r.Key
	}
	return ""
}

// reselect keeps the cursor on the same resource across refreshes.
func (m Model) reselect(key string) Model {
	rows := m.rows()
	if key != "" {
		for i, r := range rows {
			if r.Key == key {
				m.cursor = i
				m.fixScroll()
				return m
			}
		}
	}
	if m.cursor >= len(rows) {
		m.cursor = len(rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.fixScroll()
	return m
}

// drill opens the detail screen for the selected row.
func (m Model) drill() Model {
	if m.screen != screenList {
		return m
	}
	r, ok := m.selectedRow()
	if !ok {
		return m
	}
	switch m.view {
	case viewApps:
		m.detailKind, m.detailName = LogApp, r.Key
	case viewDatabases:
		m.detailKind, m.detailName = LogDatabase, r.Key
	case viewDeploys, viewAlerts:
		if r.Ref == "" {
			return m
		}
		m.detailKind, m.detailName = LogApp, r.Ref
	default:
		return m
	}
	m.screen = screenDetail
	return m
}

// contextApp is the app a bare action key or command applies to.
func (m Model) contextApp() string {
	if m.screen != screenList {
		if m.detailKind == LogApp {
			return m.detailName
		}
		return ""
	}
	r, ok := m.selectedRow()
	if !ok {
		return ""
	}
	switch m.view {
	case viewApps:
		return r.Key
	case viewDeploys, viewAlerts:
		return r.Ref
	}
	return ""
}

func (m Model) findApp(name string) (AppRow, bool) {
	for _, a := range m.apps {
		if a.Name == name {
			return a, true
		}
	}
	return AppRow{}, false
}

// rollbackTarget is the newest succeeded, non-live deploy of app.
func (m Model) rollbackTarget(app string) (DeployRow, bool) {
	var best DeployRow
	found := false
	for _, d := range m.deploys {
		if d.App != app || d.IsLive || d.Status != "succeeded" {
			continue
		}
		if !found || d.StartedAt.After(best.StartedAt) {
			best, found = d, true
		}
	}
	return best, found
}

// prepare validates a mutation and opens the typed-name confirmation.
func (m Model) prepare(kind actionKind, app string) (tea.Model, tea.Cmd) {
	if m.opts.ReadOnly {
		m.notice, m.noticeErr = readOnlyNotice, true
		return m, nil
	}
	if app == "" {
		app = m.contextApp()
	}
	row, ok := m.findApp(app)
	if app == "" || !ok {
		m.notice, m.noticeErr = "select an app first (or give its name after the command)", true
		return m, nil
	}
	p := pendingAction{kind: kind, app: app}
	switch kind {
	case actDeploy:
		if row.Image == "" {
			m.notice, m.noticeErr = app+" has no image to redeploy", true
			return m, nil
		}
		p.image = row.Image
		p.summary = "redeploy " + app + " with image " + clean(row.Image)
	case actRollback:
		t, found := m.rollbackTarget(app)
		if !found {
			m.notice, m.noticeErr = "no earlier succeeded deploy of "+app+" to roll back to", true
			return m, nil
		}
		p.deployID = t.ID
		p.summary = fmt.Sprintf("roll back %s to deploy %s (%s old)", app, shortID(t.ID, 8), age(m.opts.Now(), t.StartedAt))
	default:
		p.summary = actionVerbs[kind] + " " + app
	}
	m.pending, m.mode, m.input = &p, modeConfirm, ""
	return m, nil
}

var viewAliases = map[string]viewID{
	"apps": viewApps, "app": viewApps,
	"databases": viewDatabases, "database": viewDatabases, "db": viewDatabases,
	"deploys": viewDeploys, "deployments": viewDeploys, "deploy-history": viewDeploys,
	"nodes": viewNodes, "domains": viewDomains,
	"alerts": viewAlerts, "attention": viewAlerts,
	"backups": viewBackups,
}

var commandActions = map[string]actionKind{
	"deploy": actDeploy, "restart": actRestart, "rollback": actRollback,
	"stop": actStop, "start": actStart,
}

func (m Model) runCommand(line string) (tea.Model, tea.Cmd) {
	f := strings.Fields(line)
	if len(f) == 0 {
		return m, nil
	}
	name, arg := strings.ToLower(f[0]), ""
	if len(f) > 1 {
		arg = f[1]
	}
	if v, ok := viewAliases[name]; ok {
		return m.switchView(v), nil
	}
	if kind, ok := commandActions[name]; ok {
		return m.prepare(kind, arg)
	}
	switch name {
	case "logs", "log":
		return m.openLogs(arg)
	case "q", "quit", "exit":
		return m.quit()
	case "help", "?":
		m.mode = modeHelp
		return m, nil
	}
	m.notice, m.noticeErr = fmt.Sprintf("unknown command %q, press ? for help", name), true
	return m, nil
}
