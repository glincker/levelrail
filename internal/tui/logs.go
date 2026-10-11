package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

type logEntry struct {
	text string
	tone Tone
}

type logSession struct {
	id     int
	cancel context.CancelFunc
	ch     chan tea.Msg
}

type logsState struct {
	kind    LogKind
	name    string
	lines   []logEntry
	top     int
	follow  bool
	ended   string
	from    screen
	session *logSession
	nextID  int
}

type (
	logLineMsg struct {
		id   int
		line LogLine
	}
	logEndMsg struct {
		id  int
		err error
	}
)

const logChanSize = 64

// openLogs starts a stream for name, or the context app/database.
func (m Model) openLogs(name string) (tea.Model, tea.Cmd) {
	kind := LogApp
	if name == "" {
		switch {
		case m.screen == screenDetail:
			kind, name = m.detailKind, m.detailName
		case m.view == viewDatabases:
			kind = LogDatabase
			name = m.selectedKey()
		default:
			name = m.contextApp()
		}
	} else if _, isApp := m.findApp(name); !isApp {
		for _, d := range m.databases {
			if d.Name == name {
				kind = LogDatabase
			}
		}
	}
	if name == "" {
		m.notice, m.noticeErr = "select an app or database first", true
		return m, nil
	}
	m.stopLogs()
	m.logs.nextID++
	ctx, cancel := context.WithCancel(m.ctx)
	s := &logSession{id: m.logs.nextID, cancel: cancel, ch: make(chan tea.Msg, logChanSize)}
	api := m.api
	go func() {
		defer close(s.ch)
		err := api.Logs(ctx, kind, name, func(l LogLine) error {
			select {
			case s.ch <- logLineMsg{id: s.id, line: l}:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		if ctx.Err() == nil {
			select {
			case s.ch <- logEndMsg{id: s.id, err: err}:
			case <-ctx.Done():
			}
		}
	}()
	m.logs = logsState{
		kind: kind, name: name, follow: true, from: m.screen,
		session: s, nextID: m.logs.nextID,
	}
	m.screen = screenLogs
	return m, waitLog(s)
}

func waitLog(s *logSession) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-s.ch
		if !ok {
			return nil
		}
		return msg
	}
}

func (m *Model) stopLogs() {
	if m.logs.session != nil {
		m.logs.session.cancel()
		m.logs.session = nil
	}
}

func (m Model) handleLogLine(msg logLineMsg) (tea.Model, tea.Cmd) {
	s := m.logs.session
	if s == nil || s.id != msg.id {
		return m, nil
	}
	text := clean(msg.line.Text)
	lines := append(m.logs.lines, logEntry{text: text, tone: logTone(text, msg.line.Stream)})
	if over := len(lines) - m.opts.LogBuffer; over > 0 {
		lines = append([]logEntry(nil), lines[over:]...)
		if m.logs.top -= over; m.logs.top < 0 {
			m.logs.top = 0
		}
	}
	m.logs.lines = lines
	return m, waitLog(s)
}

func (m Model) handleLogEnd(msg logEndMsg) Model {
	if m.logs.session == nil || m.logs.session.id != msg.id {
		return m
	}
	m.logs.ended = "stream closed"
	if msg.err != nil {
		m.logs.ended = "stream ended: " + clean(msg.err.Error())
	}
	m.logs.session = nil
	return m
}

// logHeight is the number of log lines that fit the body.
func (m Model) logHeight() int {
	if h := m.h - 4; h > 1 {
		return h
	}
	return 1
}

func (m Model) keyLogs(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	maxTop := len(m.logs.lines) - m.logHeight()
	if maxTop < 0 {
		maxTop = 0
	}
	cur := m.logs.top
	if m.logs.follow {
		cur = maxTop
	}
	scroll := func(to int) Model {
		if to < 0 {
			to = 0
		}
		m.logs.follow = false
		if to >= maxTop {
			to, m.logs.follow = maxTop, true
		}
		m.logs.top = to
		return m
	}
	switch msg.String() {
	case "q", "esc":
		return m.back(), nil
	case "?":
		m.mode = modeHelp
	case ":":
		m.mode, m.input = modeCommand, ""
	case "j", "down":
		return scroll(cur + 1), nil
	case "k", "up":
		return scroll(cur - 1), nil
	case "ctrl+d", "pgdown":
		return scroll(cur + m.logHeight()/2), nil
	case "ctrl+u", "pgup":
		return scroll(cur - m.logHeight()/2), nil
	case "g", "home":
		return scroll(0), nil
	case "G", "end":
		m.logs.follow = true
	case "f":
		m.logs.follow = !m.logs.follow
		if !m.logs.follow {
			m.logs.top = cur
		}
	case "c":
		m.logs.lines, m.logs.top = nil, 0
	}
	return m, nil
}
