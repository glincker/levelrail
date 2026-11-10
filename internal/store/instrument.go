package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

// timedDriverName is the database/sql driver every store handle opens with; it forwards to the sqlite driver.
const timedDriverName = "sqlite-timed"

// StatementTimingConfig is the live statement timing setup, fed from runtime settings.
type StatementTimingConfig struct {
	Enabled  bool
	SlowMS   int
	RingSize int
}

// SlowStatement is one statement text in the slowest statements ring. Bound values are never recorded.
type SlowStatement struct {
	SQL        string    `json:"sql"`
	MaxMS      float64   `json:"max_ms"`
	LastMS     float64   `json:"last_ms"`
	Count      int64     `json:"count"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

// StatementStats accumulates the statements one request ran.
type StatementStats struct {
	queries    atomic.Int64
	nanos      atomic.Int64
	privileged atomic.Bool
}

// Snapshot returns the statement count and total statement time so far.
func (s *StatementStats) Snapshot() (int64, time.Duration) {
	if s == nil {
		return 0, 0
	}
	return s.queries.Load(), time.Duration(s.nanos.Load())
}

// MarkPrivileged records that the auth gate admitted a caller allowed to see timing; nil is a no-op.
func (s *StatementStats) MarkPrivileged() {
	if s != nil {
		s.privileged.Store(true)
	}
}

// Privileged reports whether MarkPrivileged was called.
func (s *StatementStats) Privileged() bool { return s != nil && s.privileged.Load() }

type statsKey struct{}

// WithStatementStats attaches a fresh accumulator that every statement run with the returned context adds to.
func WithStatementStats(ctx context.Context) (context.Context, *StatementStats) {
	st := &StatementStats{}
	return context.WithValue(ctx, statsKey{}, st), st
}

// StatementStatsFrom returns the accumulator on ctx, or nil when timing is not attached.
func StatementStatsFrom(ctx context.Context) *StatementStats {
	st, _ := ctx.Value(statsKey{}).(*StatementStats)
	return st
}

type statementTiming struct {
	on     atomic.Bool
	slowNS atomic.Int64
	mu     sync.Mutex
	size   int
	ring   []SlowStatement
	logger atomic.Pointer[slog.Logger]
}

// One process, one control plane database: the timing state is process wide like the driver registry.
var timing = &statementTiming{size: 20}

// SetStatementTiming applies a timing config; it is cheap enough to call on every request.
func SetStatementTiming(cfg StatementTimingConfig) {
	timing.on.Store(cfg.Enabled)
	if cfg.SlowMS > 0 {
		timing.slowNS.Store(int64(cfg.SlowMS) * int64(time.Millisecond))
	}
	if cfg.RingSize > 0 {
		timing.mu.Lock()
		timing.size = cfg.RingSize
		timing.trimLocked()
		timing.mu.Unlock()
	}
}

// StatementTimingEnabled reports whether the timing layer is recording.
func StatementTimingEnabled() bool { return timing.on.Load() }

// SetStatementLogger sets where slow statements are logged; nil goes back to slog.Default.
func SetStatementLogger(l *slog.Logger) { timing.logger.Store(l) }

// SlowStatements returns the ring, slowest first.
func SlowStatements() []SlowStatement {
	timing.mu.Lock()
	out := append([]SlowStatement(nil), timing.ring...)
	timing.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].MaxMS > out[j].MaxMS })
	return out
}

// ResetSlowStatements empties the ring.
func ResetSlowStatements() {
	timing.mu.Lock()
	timing.ring = nil
	timing.mu.Unlock()
}

func (t *statementTiming) trimLocked() {
	for len(t.ring) > t.size {
		lo := fastestIndex(t.ring)
		t.ring = append(t.ring[:lo], t.ring[lo+1:]...)
	}
}

func fastestIndex(r []SlowStatement) int {
	lo := 0
	for i := range r {
		if r[i].MaxMS < r[lo].MaxMS {
			lo = i
		}
	}
	return lo
}

func (t *statementTiming) record(ctx context.Context, query string, d time.Duration) {
	if st, _ := ctx.Value(statsKey{}).(*StatementStats); st != nil {
		st.queries.Add(1)
		st.nanos.Add(int64(d))
	}
	slow := t.slowNS.Load()
	if slow <= 0 || int64(d) < slow {
		return
	}
	text := redactStatement(query)
	ms := float64(d) / float64(time.Millisecond)
	l := t.logger.Load()
	if l == nil {
		l = slog.Default()
	}
	l.Warn("store: slow statement", slog.String("statement", text), slog.Float64("duration_ms", ms))
	t.remember(text, ms)
}

func (t *statementTiming) remember(text string, ms float64) {
	now := time.Now().UTC()
	t.mu.Lock()
	defer t.mu.Unlock()
	for i := range t.ring {
		if t.ring[i].SQL == text {
			e := &t.ring[i]
			e.Count++
			e.LastMS, e.LastSeenAt = ms, now
			e.MaxMS = max(e.MaxMS, ms)
			return
		}
	}
	if len(t.ring) >= t.size {
		lo := fastestIndex(t.ring)
		if t.ring[lo].MaxMS >= ms {
			return
		}
		t.ring = append(t.ring[:lo], t.ring[lo+1:]...)
	}
	t.ring = append(t.ring, SlowStatement{SQL: text, MaxMS: ms, LastMS: ms, Count: 1, LastSeenAt: now})
}

// redactStatement keeps the prepared text only: comments go, and quoted strings, blobs and numbers
// become ? so an inlined value never reaches a log or the ring.
func redactStatement(query string) string {
	var b strings.Builder
	b.Grow(len(query))
	for i := 0; i < len(query); {
		c := query[i]
		switch {
		case c == '-' && i+1 < len(query) && query[i+1] == '-':
			for i < len(query) && query[i] != '\n' {
				i++
			}
			b.WriteByte(' ')
		case c == '/' && i+1 < len(query) && query[i+1] == '*':
			end := strings.Index(query[i+2:], "*/")
			if end < 0 {
				i = len(query)
			} else {
				i += end + 4
			}
			b.WriteByte(' ')
		case c == '\'' || c == '"':
			i = skipQuoted(query, i, c)
			b.WriteByte('?')
		case (c == 'x' || c == 'X') && i+1 < len(query) && query[i+1] == '\'' && !identByte(prevByte(query, i)):
			i = skipQuoted(query, i+1, '\'')
			b.WriteByte('?')
		case isDigit(c) && !identByte(prevByte(query, i)) && prevByte(query, i) != '?':
			for i < len(query) && (identByte(query[i]) || query[i] == '.') {
				i++
			}
			b.WriteByte('?')
		default:
			b.WriteByte(c)
			i++
		}
	}
	q := strings.Join(strings.Fields(b.String()), " ")
	if len(q) > maxLoggedStatement {
		cut := maxLoggedStatement
		for cut > 0 && !utf8.RuneStart(q[cut]) {
			cut--
		}
		q = q[:cut]
	}
	return q
}

// skipQuoted returns the index after the quoted run starting at open; a doubled quote is an escape.
func skipQuoted(s string, open int, q byte) int {
	for i := open + 1; i < len(s); i++ {
		if s[i] != q {
			continue
		}
		if i+1 < len(s) && s[i+1] == q {
			i++
			continue
		}
		return i + 1
	}
	return len(s)
}

func prevByte(s string, i int) byte {
	if i == 0 {
		return ' '
	}
	return s[i-1]
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func identByte(c byte) bool {
	return c == '_' || isDigit(c) || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

type timedDriver struct{ inner driver.Driver }

func init() {
	probe, err := sql.Open("sqlite", "")
	if err != nil {
		panic("store: sqlite driver missing: " + err.Error())
	}
	inner := probe.Driver()
	_ = probe.Close()
	sql.Register(timedDriverName, timedDriver{inner: inner})
}

func (d timedDriver) Open(name string) (driver.Conn, error) {
	c, err := d.inner.Open(name)
	if err != nil {
		return nil, err
	}
	full, ok := c.(sqliteConn)
	if !ok {
		return c, nil
	}
	return &timedConn{c: full}, nil
}

// sqliteConn is the set of optional interfaces the sqlite driver implements; the wrapper claims exactly these.
type sqliteConn interface {
	driver.Conn
	driver.ConnBeginTx
	driver.ConnPrepareContext
	driver.ExecerContext
	driver.QueryerContext
	driver.Pinger
	driver.SessionResetter
	driver.Validator
}

type timedConn struct{ c sqliteConn }

func (t *timedConn) Prepare(query string) (driver.Stmt, error) {
	return t.PrepareContext(context.Background(), query)
}

func (t *timedConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	s, err := t.c.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	full, ok := s.(sqliteStmt)
	if !ok {
		return s, nil
	}
	return &timedStmt{s: full, query: query}, nil
}

func (t *timedConn) Close() error                           { return t.c.Close() }
func (t *timedConn) Begin() (driver.Tx, error)              { return t.c.Begin() } //nolint:staticcheck // driver.Conn requires Begin
func (t *timedConn) Ping(ctx context.Context) error         { return t.c.Ping(ctx) }
func (t *timedConn) ResetSession(ctx context.Context) error { return t.c.ResetSession(ctx) }
func (t *timedConn) IsValid() bool                          { return t.c.IsValid() }

func (t *timedConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	tx, err := t.c.BeginTx(ctx, opts)
	if err != nil || !timing.on.Load() {
		return tx, err
	}
	return &timedTx{tx: tx, ctx: ctx}, nil
}

// timedTx bills COMMIT and ROLLBACK (fsync, WAL lock waits) to the context that began the transaction.
type timedTx struct {
	tx  driver.Tx
	ctx context.Context //nolint:containedctx // driver.Tx.Commit takes no context
}

func (t *timedTx) Commit() error {
	start := time.Now()
	err := t.tx.Commit()
	timing.record(t.ctx, "COMMIT", time.Since(start))
	return err
}

func (t *timedTx) Rollback() error {
	start := time.Now()
	err := t.tx.Rollback()
	timing.record(t.ctx, "ROLLBACK", time.Since(start))
	return err
}

func (t *timedConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if !timing.on.Load() {
		return t.c.ExecContext(ctx, query, args)
	}
	start := time.Now()
	res, err := t.c.ExecContext(ctx, query, args)
	timing.record(ctx, query, time.Since(start))
	return res, err
}

func (t *timedConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if !timing.on.Load() {
		return t.c.QueryContext(ctx, query, args)
	}
	start := time.Now()
	rows, err := t.c.QueryContext(ctx, query, args)
	timing.record(ctx, query, time.Since(start))
	return rows, err
}

type sqliteStmt interface {
	driver.Stmt
	driver.StmtExecContext
	driver.StmtQueryContext
}

type timedStmt struct {
	s     sqliteStmt
	query string
}

func (t *timedStmt) Close() error  { return t.s.Close() }
func (t *timedStmt) NumInput() int { return t.s.NumInput() }

//nolint:staticcheck // driver.Stmt requires the legacy methods
func (t *timedStmt) Exec(args []driver.Value) (driver.Result, error) { return t.s.Exec(args) }

//nolint:staticcheck // driver.Stmt requires the legacy methods
func (t *timedStmt) Query(args []driver.Value) (driver.Rows, error) { return t.s.Query(args) }

func (t *timedStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if !timing.on.Load() {
		return t.s.ExecContext(ctx, args)
	}
	start := time.Now()
	res, err := t.s.ExecContext(ctx, args)
	timing.record(ctx, t.query, time.Since(start))
	return res, err
}

func (t *timedStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	if !timing.on.Load() {
		return t.s.QueryContext(ctx, args)
	}
	start := time.Now()
	rows, err := t.s.QueryContext(ctx, args)
	timing.record(ctx, t.query, time.Since(start))
	return rows, err
}
