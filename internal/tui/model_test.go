package tui

import (
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

var update = flag.Bool("update", false, "rewrite golden files")

var fixedNow = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

type fakeAPI struct {
	mu       sync.Mutex
	fast     Fast
	slow     Slow
	calls    []string
	actErr   error
	logLines []LogLine
}

func (f *fakeAPI) Fast(context.Context) Fast { return f.fast }
func (f *fakeAPI) Slow(context.Context) Slow { return f.slow }

func (f *fakeAPI) record(s string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, s)
	return f.actErr
}

func (f *fakeAPI) Restart(_ context.Context, app string) error { return f.record("restart " + app) }
func (f *fakeAPI) Start(_ context.Context, app string) error   { return f.record("start " + app) }
func (f *fakeAPI) Stop(_ context.Context, app string) error    { return f.record("stop " + app) }
func (f *fakeAPI) Deploy(_ context.Context, app, image string) error {
	return f.record("deploy " + app + " " + image)
}
func (f *fakeAPI) Rollback(_ context.Context, app, id string) error {
	return f.record("rollback " + app + " " + id)
}

func (f *fakeAPI) Logs(ctx context.Context, _ LogKind, _ string, on func(LogLine) error) error {
	for _, l := range f.logLines {
		if err := on(l); err != nil {
			return err
		}
	}
	<-ctx.Done()
	return ctx.Err()
}

func newFake() *fakeAPI {
	return &fakeAPI{
		fast: Fast{
			Apps: []AppRow{
				{Name: "api", Status: "Healthy", Tone: ToneSuccess, Image: "reg/api:1.4", Replicas: 2, Strategy: "rolling", Domains: []string{"api.example.com", "b.example.com"}},
				{Name: "web", Status: "Attention needed", Tone: ToneDanger, Image: "reg/web:2.0", Node: "n1"},
				{Name: "worker", Status: "Stopped", Tone: ToneNeutral, Image: "reg/worker:3"},
			},
			Deploys: []DeployRow{
				{ID: "d-web-new", App: "web", Status: "failed", Trigger: "git", Env: "prod", Commit: "abcdef123", Branch: "main", StartedAt: fixedNow.Add(-5 * time.Minute), DurationMS: 41000},
				{ID: "d-web-good", App: "web", Status: "succeeded", Trigger: "git", Env: "prod", Commit: "1234567890", Branch: "main", StartedAt: fixedNow.Add(-3 * time.Hour), DurationMS: 90000},
				{ID: "d-web-live", App: "web", Status: "succeeded", IsLive: true, StartedAt: fixedNow.Add(-1 * time.Hour)},
				{ID: "d-api-1", App: "api", Status: "succeeded", IsLive: true, StartedAt: fixedNow.Add(-26 * time.Hour)},
			},
			Attention: []AttentionRow{
				{Severity: "critical", Kind: "app_failing", Subject: "web", Title: "web is failing", Detail: "health check"},
				{Severity: "warning", Kind: "cert", Subject: "api.example.com", Title: "certificate expires soon"},
			},
		},
		slow: Slow{
			Databases: []DBRow{{Name: "main", Engine: "postgres", Version: "16", Schedule: "0 3 * * *", Retain: 7}},
			Nodes:     []NodeRow{{Name: "n1", Address: "10.0.0.2", Status: "online", Schedulable: true, Local: true, LastSeen: fixedNow.Add(-4 * time.Second)}},
			Domains:   []DomainRow{{Domain: "api.example.com", App: "api", TLS: "acme", Kind: "app"}},
			Backups:   []BackupRow{{Resource: "main", Kind: "database", Status: "succeeded", SizeBytes: 5 << 20, StartedAt: fixedNow.Add(-9 * time.Hour)}},
			Mesh:      MeshInfo{Enabled: true, Peers: 3, Healthy: 3},
			Server:    ServerInfo{Version: "v0.6.1"},
		},
	}
}

func newTestModel(t *testing.T, api API, readOnly bool) Model {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	m := New(ctx, api, Options{Prog: "lr", Profile: "default", Host: "localhost:9100", ReadOnly: readOnly, Now: func() time.Time { return fixedNow }})
	m.w, m.h = 100, 20
	f, s := api.Fast(ctx), api.Slow(ctx)
	return m.applyFast(f).applySlow(s)
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func press(m Model, keys ...string) (Model, tea.Cmd) {
	var cmd tea.Cmd
	for _, k := range keys {
		var next tea.Model
		next, cmd = m.Update(key(k))
		m = next.(Model)
	}
	return m, cmd
}

func typeText(m Model, s string) Model {
	for _, r := range s {
		m, _ = press(m, string(r))
	}
	return m
}

func TestNavigationKeys(t *testing.T) {
	tests := []struct {
		name       string
		keys       []string
		wantView   viewID
		wantScreen screen
		wantCursor int
	}{
		{"down moves", []string{"j"}, viewApps, screenList, 1},
		{"clamps at bottom", []string{"G", "j"}, viewApps, screenList, 2},
		{"up clamps at top", []string{"k"}, viewApps, screenList, 0},
		{"number switches", []string{"3"}, viewDeploys, screenList, 0},
		{"tab cycles", []string{"tab"}, viewDatabases, screenList, 0},
		{"enter drills", []string{"j", "enter"}, viewApps, screenDetail, 1},
		{"esc returns", []string{"enter", "esc"}, viewApps, screenList, 0},
		{"colon command switches", []string{":", "n", "o", "d", "e", "s", "enter"}, viewNodes, screenList, 0},
		{"unknown command stays", []string{":", "z", "enter"}, viewApps, screenList, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(t, newFake(), false)
			m, _ = press(m, tc.keys...)
			if m.view != tc.wantView || m.screen != tc.wantScreen || m.cursor != tc.wantCursor {
				t.Fatalf("view=%d screen=%d cursor=%d, want %d/%d/%d", m.view, m.screen, m.cursor, tc.wantView, tc.wantScreen, tc.wantCursor)
			}
		})
	}
}

func TestFilter(t *testing.T) {
	m := newTestModel(t, newFake(), false)
	m, _ = press(m, "/")
	m = typeText(m, "wor")
	if got := len(m.rows()); got != 1 || m.rows()[0].Key != "worker" {
		t.Fatalf("rows after filter = %v", m.rows())
	}
	m, _ = press(m, "enter")
	if m.mode != modeNormal || m.filter != "wor" {
		t.Fatalf("enter should keep filter, mode=%d filter=%q", m.mode, m.filter)
	}
	m, _ = press(m, "esc")
	if m.filter != "" || len(m.rows()) != 3 {
		t.Fatalf("esc should clear filter, got %q (%d rows)", m.filter, len(m.rows()))
	}
}

func TestConfirmGatedActions(t *testing.T) {
	tests := []struct {
		name     string
		keys     []string
		typed    string
		wantCall string
		wantMsg  string
	}{
		{"restart with name", []string{"r"}, "api", "restart api", ""},
		{"restart wrong name", []string{"r"}, "ap", "", "name did not match"},
		{"deploy via command", []string{":", "d", "e", "p", "l", "o", "y", "space", "w", "e", "b", "enter"}, "web", "deploy web reg/web:2.0", ""},
		{"rollback picks newest succeeded non-live", []string{"j", "b"}, "web", "rollback web d-web-good", ""},
		{"rollback without target", []string{"b"}, "api", "", "no earlier succeeded deploy"},
		{"stop", []string{"x"}, "api", "stop api", ""},
		{"start", []string{"j", "j", "o"}, "worker", "start worker", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			api := newFake()
			m := newTestModel(t, api, false)
			m, _ = press(m, tc.keys...)
			if m.mode != modeConfirm {
				if !strings.Contains(m.notice, tc.wantMsg) || tc.wantCall != "" {
					t.Fatalf("mode=%d notice=%q, want confirm prompt", m.mode, m.notice)
				}
				return
			}
			m = typeText(m, tc.typed)
			var cmd tea.Cmd
			m, cmd = press(m, "enter")
			if tc.wantCall == "" {
				if cmd != nil || !strings.Contains(m.notice, tc.wantMsg) {
					t.Fatalf("expected rejection %q, notice=%q cmd=%v", tc.wantMsg, m.notice, cmd != nil)
				}
				if len(api.calls) != 0 {
					t.Fatalf("api called on mismatch: %v", api.calls)
				}
				return
			}
			if cmd == nil {
				t.Fatalf("no command returned, notice=%q", m.notice)
			}
			msg := cmd().(actionMsg)
			if msg.err != nil {
				t.Fatal(msg.err)
			}
			if len(api.calls) != 1 || api.calls[0] != tc.wantCall {
				t.Fatalf("calls = %v, want %q", api.calls, tc.wantCall)
			}
		})
	}
}

func TestEscapeCancelsConfirm(t *testing.T) {
	api := newFake()
	m := newTestModel(t, api, false)
	m, _ = press(m, "r")
	m = typeText(m, "api")
	m, cmd := press(m, "esc")
	if m.mode != modeNormal || m.pending != nil || cmd != nil || len(api.calls) != 0 {
		t.Fatalf("esc must cancel without calling the API (mode=%d calls=%v)", m.mode, api.calls)
	}
}

func TestReadOnlyBlocksEveryMutation(t *testing.T) {
	for _, k := range []string{"r", "d", "b", "x", "o"} {
		t.Run(k, func(t *testing.T) {
			api := newFake()
			m := newTestModel(t, api, true)
			m, cmd := press(m, k)
			if m.mode == modeConfirm || cmd != nil || !m.noticeErr || !strings.Contains(m.notice, "read-only") {
				t.Fatalf("mutation key %q not blocked: mode=%d notice=%q", k, m.mode, m.notice)
			}
		})
	}
	m := newTestModel(t, newFake(), true)
	m, _ = press(m, ":", "r", "e", "s", "t", "a", "r", "t", "enter")
	if m.mode == modeConfirm {
		t.Fatal(":restart opened a confirmation in read-only mode")
	}
	if !strings.Contains(m.View(), "[READ-ONLY]") {
		t.Fatal("header must show read-only banner")
	}
	if strings.Contains(m.hintLine(), "restart") {
		t.Fatal("read-only hints must not advertise mutations")
	}
}

func TestActionErrorSurfacesAndSuccessRefreshes(t *testing.T) {
	api := newFake()
	m := newTestModel(t, api, false)
	next, cmd := m.Update(actionMsg{label: "restart api", err: errors.New("boom \x1b[31mred")})
	m = next.(Model)
	if !m.noticeErr || strings.Contains(m.notice, "\x1b") || cmd != nil {
		t.Fatalf("error not shown cleanly: %q", m.notice)
	}
	next, cmd = m.Update(actionMsg{label: "restart api"})
	m = next.(Model)
	if m.noticeErr || cmd == nil || !m.fastInFlight {
		t.Fatalf("success should trigger a refresh: notice=%q", m.notice)
	}
}

func TestTickSkipsOverlappingFetches(t *testing.T) {
	m := newTestModel(t, newFake(), false)
	m.fastInFlight = true
	next, _ := m.Update(tickMsg(fixedNow))
	m = next.(Model)
	if !m.fastInFlight || m.ticks != 1 {
		t.Fatalf("tick bookkeeping wrong: %+v", m.ticks)
	}
	m.fastInFlight, m.slowInFlight = false, false
	m.ticks = m.opts.SlowEvery - 1
	next, _ = m.Update(tickMsg(fixedNow))
	m = next.(Model)
	if !m.fastInFlight || !m.slowInFlight {
		t.Fatal("slow section should refresh every SlowEvery ticks")
	}
}

func TestFailedSectionKeepsStaleData(t *testing.T) {
	m := newTestModel(t, newFake(), false)
	m = m.applyFast(Fast{Errs: map[string]string{SecApps: "boom"}})
	if len(m.apps) != 3 || m.errs[SecApps] != "boom" {
		t.Fatalf("stale apps lost or error missing: %d %v", len(m.apps), m.errs)
	}
	m = m.applyFast(Fast{Apps: m.apps})
	if _, bad := m.errs[SecApps]; bad {
		t.Fatal("error should clear on next success")
	}
}

func TestSelectionFollowsResourceAcrossRefresh(t *testing.T) {
	m := newTestModel(t, newFake(), false)
	m, _ = press(m, "j")
	f := newFake().fast
	f.Apps = append([]AppRow{{Name: "aaa", Status: "Healthy"}}, f.Apps...)
	m = m.applyFast(f)
	if r, _ := m.selectedRow(); r.Key != "web" {
		t.Fatalf("cursor moved off web to %q", r.Key)
	}
}

func TestLogsStreamColourAndBack(t *testing.T) {
	api := newFake()
	api.logLines = []LogLine{
		{Text: "INFO started"}, {Text: "WARN slow query"},
		{Text: "ERROR \x1b[2Jboom"}, {Text: "plain"},
	}
	m := newTestModel(t, api, false)
	m, cmd := press(m, "l")
	if m.screen != screenLogs || cmd == nil {
		t.Fatalf("screen=%d cmd=%v", m.screen, cmd != nil)
	}
	for i := 0; i < 4; i++ {
		msg := cmd()
		next, c := m.Update(msg)
		m, cmd = next.(Model), c
	}
	if len(m.logs.lines) != 4 {
		t.Fatalf("lines = %d", len(m.logs.lines))
	}
	wantTones := []Tone{ToneInfo, ToneWarning, ToneDanger, toneNone}
	for i, w := range wantTones {
		if m.logs.lines[i].tone != w {
			t.Errorf("line %d tone = %d, want %d", i, m.logs.lines[i].tone, w)
		}
	}
	if strings.Contains(m.logs.lines[2].text, "\x1b") {
		t.Fatal("escape sequence leaked into log line")
	}
	m, _ = press(m, "esc")
	if m.screen != screenList || m.logs.session != nil {
		t.Fatal("esc must close the stream and return")
	}
}

func TestLogBufferCap(t *testing.T) {
	m := newTestModel(t, newFake(), false)
	m.opts.LogBuffer = 3
	s := &logSession{id: 1, cancel: func() {}, ch: make(chan tea.Msg, 1)}
	m.logs.session = s
	for i := 0; i < 5; i++ {
		next, _ := m.Update(logLineMsg{id: 1, line: LogLine{Text: "l"}})
		m = next.(Model)
	}
	if len(m.logs.lines) != 3 {
		t.Fatalf("buffer not capped: %d", len(m.logs.lines))
	}
	next, _ := m.Update(logLineMsg{id: 99, line: LogLine{Text: "stale"}})
	if len(next.(Model).logs.lines) != 3 {
		t.Fatal("stale session line accepted")
	}
}

func TestCleanStripsControlSequences(t *testing.T) {
	got := clean("a\x1b[31mb\x1b]0;title\x07c\x00d\te")
	if got != "abcd e" {
		t.Fatalf("clean = %q", got)
	}
}

func TestViewGolden(t *testing.T) {
	cases := map[string]func(Model) Model{
		"apps": func(m Model) Model { return m },
		"deploys": func(m Model) Model {
			m, _ = press(m, "3")
			return m
		},
		"app_detail": func(m Model) Model {
			m, _ = press(m, "j", "enter")
			return m
		},
		"alerts": func(m Model) Model {
			m, _ = press(m, "6")
			return m
		},
		"confirm": func(m Model) Model {
			m, _ = press(m, "r")
			return typeText(m, "ap")
		},
		"help": func(m Model) Model {
			m, _ = press(m, "?")
			return m
		},
		"empty_apps": func(m Model) Model {
			return m.applyFast(Fast{Apps: []AppRow{}})
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			m := setup(newTestModel(t, newFake(), false))
			got := m.View() + "\n"
			path := filepath.Join("testdata", name+".golden")
			if *update {
				if err := os.MkdirAll("testdata", 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path) //nolint:gosec // fixed testdata path
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Errorf("view mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
			}
			for i, line := range strings.Split(strings.TrimRight(got, "\n"), "\n") {
				if width(line) > m.w {
					t.Errorf("line %d wider than terminal: %d", i, width(line))
				}
			}
		})
	}
}

func TestViewHeightIsStable(t *testing.T) {
	m := newTestModel(t, newFake(), false)
	for _, h := range []int{8, 12, 40} {
		m.h = h
		if got := strings.Count(m.View(), "\n") + 1; got != h {
			t.Errorf("height %d rendered %d lines", h, got)
		}
	}
	m.w = 20
	if !strings.Contains(m.View(), "too small") {
		t.Fatal("tiny terminal should show a hint")
	}
}
