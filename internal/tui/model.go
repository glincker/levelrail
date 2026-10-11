package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	defaultRefresh   = 2 * time.Second
	defaultSlowEvery = 5
	defaultLogBuffer = 2000
	actionTimeout    = 60 * time.Second
)

// Options configures a Model. Zero values pick sane defaults.
type Options struct {
	Prog      string
	Profile   string
	Host      string
	ReadOnly  bool
	Color     bool
	Refresh   time.Duration
	SlowEvery int
	LogBuffer int
	Now       func() time.Time
}

func (o Options) withDefaults() Options {
	if o.Refresh <= 0 {
		o.Refresh = defaultRefresh
	}
	if o.SlowEvery <= 0 {
		o.SlowEvery = defaultSlowEvery
	}
	if o.LogBuffer <= 0 {
		o.LogBuffer = defaultLogBuffer
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return o
}

type viewID int

const (
	viewApps viewID = iota
	viewDatabases
	viewDeploys
	viewNodes
	viewDomains
	viewAlerts
	viewBackups
	viewCount
)

var viewNames = [viewCount]string{"Apps", "Databases", "Deploys", "Nodes", "Domains", "Alerts", "Backups"}

type screen int

const (
	screenList screen = iota
	screenDetail
	screenLogs
)

type inputMode int

const (
	modeNormal inputMode = iota
	modeFilter
	modeCommand
	modeConfirm
	modeHelp
)

type actionKind int

const (
	actRestart actionKind = iota
	actStart
	actStop
	actDeploy
	actRollback
)

var actionVerbs = map[actionKind]string{
	actRestart: "restart", actStart: "start", actStop: "stop",
	actDeploy: "deploy", actRollback: "rollback",
}

type pendingAction struct {
	kind     actionKind
	app      string
	image    string
	deployID string
	summary  string
}

type (
	fastMsg   struct{ data Fast }
	slowMsg   struct{ data Slow }
	tickMsg   time.Time
	actionMsg struct {
		label string
		err   error
	}
)

// Model is the Bubble Tea model for the whole UI.
type Model struct {
	api  API
	opts Options
	st   styles
	ctx  context.Context

	w, h int

	view   viewID
	screen screen
	mode   inputMode
	cursor int
	offset int
	filter string
	input  string

	detailKind LogKind
	detailName string

	apps      []AppRow
	deploys   []DeployRow
	attention []AttentionRow
	databases []DBRow
	nodes     []NodeRow
	domains   []DomainRow
	backups   []BackupRow
	mesh      MeshInfo
	server    ServerInfo
	errs      map[string]string

	fastLoaded, slowLoaded     bool
	fastInFlight, slowInFlight bool
	ticks                      int

	notice    string
	noticeErr bool
	pending   *pendingAction

	logs logsState
}

// New builds a Model; ctx bounds every background call it makes.
func New(ctx context.Context, api API, opts Options) Model {
	opts = opts.withDefaults()
	return Model{
		api: api, opts: opts, st: newStyles(opts.Color), ctx: ctx,
		w: 100, h: 30, errs: map[string]string{},
		logs: logsState{follow: true},
	}
}

// Init starts the first fetches and the refresh ticker.
func (m Model) Init() tea.Cmd {
	m.fastInFlight, m.slowInFlight = true, true
	return tea.Batch(m.fetchFast(), m.fetchSlow(), m.tick())
}

func (m Model) tick() tea.Cmd {
	return tea.Tick(m.opts.Refresh, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) fetchFast() tea.Cmd {
	api, ctx := m.api, m.ctx
	return func() tea.Msg { return fastMsg{data: api.Fast(ctx)} }
}

func (m Model) fetchSlow() tea.Cmd {
	api, ctx := m.api, m.ctx
	return func() tea.Msg { return slowMsg{data: api.Slow(ctx)} }
}

// Update routes messages to the handlers below.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.fixScroll()
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tickMsg:
		return m.handleTick()
	case fastMsg:
		return m.applyFast(msg.data), nil
	case slowMsg:
		return m.applySlow(msg.data), nil
	case actionMsg:
		return m.handleAction(msg)
	case logLineMsg:
		return m.handleLogLine(msg)
	case logEndMsg:
		return m.handleLogEnd(msg), nil
	}
	return m, nil
}

func (m Model) handleTick() (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{m.tick()}
	m.ticks++
	if !m.fastInFlight {
		m.fastInFlight = true
		cmds = append(cmds, m.fetchFast())
	}
	if m.ticks%m.opts.SlowEvery == 0 && !m.slowInFlight {
		m.slowInFlight = true
		cmds = append(cmds, m.fetchSlow())
	}
	return m, tea.Batch(cmds...)
}

func (m Model) applyFast(d Fast) Model {
	key := m.selectedKey()
	m.fastInFlight, m.fastLoaded = false, true
	m.errs = copyErrs(m.errs)
	m.section(d.Errs, SecApps, func() { m.apps = d.Apps })
	m.section(d.Errs, SecDeploys, func() { m.deploys = d.Deploys })
	m.section(d.Errs, SecAttention, func() { m.attention = d.Attention })
	return m.reselect(key)
}

func (m Model) applySlow(d Slow) Model {
	key := m.selectedKey()
	m.slowInFlight, m.slowLoaded = false, true
	m.errs = copyErrs(m.errs)
	m.section(d.Errs, SecDatabases, func() { m.databases = d.Databases })
	m.section(d.Errs, SecNodes, func() { m.nodes = d.Nodes })
	m.section(d.Errs, SecDomains, func() { m.domains = d.Domains })
	m.section(d.Errs, SecBackups, func() { m.backups = d.Backups })
	m.section(d.Errs, SecMesh, func() { m.mesh = d.Mesh })
	m.section(d.Errs, SecServer, func() { m.server = d.Server })
	return m.reselect(key)
}

// section records a failed section's error and keeps its previous data,
// or clears the error and applies fresh data.
func (m *Model) section(errs map[string]string, sec string, apply func()) {
	if msg, bad := errs[sec]; bad {
		m.errs[sec] = msg
		return
	}
	delete(m.errs, sec)
	apply()
}

func copyErrs(in map[string]string) map[string]string {
	out := make(map[string]string, len(in)+1)
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (m Model) handleAction(msg actionMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.notice, m.noticeErr = clean(msg.err.Error()), true
		return m, nil
	}
	m.notice, m.noticeErr = msg.label+" requested", false
	m.fastInFlight = true
	return m, m.fetchFast()
}

func (m Model) runAction(p pendingAction) tea.Cmd {
	api, ctx := m.api, m.ctx
	return func() tea.Msg {
		c, cancel := context.WithTimeout(ctx, actionTimeout)
		defer cancel()
		var err error
		switch p.kind {
		case actRestart:
			err = api.Restart(c, p.app)
		case actStart:
			err = api.Start(c, p.app)
		case actStop:
			err = api.Stop(c, p.app)
		case actDeploy:
			err = api.Deploy(c, p.app, p.image)
		case actRollback:
			err = api.Rollback(c, p.app, p.deployID)
		}
		return actionMsg{label: fmt.Sprintf("%s %s", actionVerbs[p.kind], p.app), err: err}
	}
}
