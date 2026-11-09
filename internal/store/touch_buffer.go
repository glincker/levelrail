package store

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"
)

// defaultTouchFlushInterval seeds the write-behind window; LEVELRAIL_TOUCH_FLUSH_INTERVAL overrides it, and 0 writes through.
const defaultTouchFlushInterval = 5 * time.Second

const touchTimeLayout = "2006-01-02T15:04:05.000Z"

type sessionTouch struct {
	userID string
	at     time.Time
}

// touchBuffer coalesces last-seen and last-used writes so hot paths stop taking the single writer connection inline.
type touchBuffer struct {
	mu       sync.Mutex
	tokens   map[string]time.Time
	nodes    map[string]time.Time
	sessions map[string]sessionTouch
	interval time.Duration
	start    sync.Once
	stop     chan struct{}
	stopOnce sync.Once
	done     chan struct{}
}

func newTouchBuffer() *touchBuffer {
	iv := defaultTouchFlushInterval
	if raw := strings.TrimSpace(os.Getenv("LEVELRAIL_TOUCH_FLUSH_INTERVAL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d >= 0 {
			iv = d
		}
	}
	return &touchBuffer{
		tokens: map[string]time.Time{}, nodes: map[string]time.Time{}, sessions: map[string]sessionTouch{},
		interval: iv, stop: make(chan struct{}), done: make(chan struct{}),
	}
}

// touch records the newest timestamp per id; the flusher goroutine starts on first use.
func (db *DB) touch(kind func(*touchBuffer) map[string]time.Time, id string, write func() error) error {
	b := db.touches
	if b == nil || b.interval == 0 {
		return write()
	}
	b.mu.Lock()
	kind(b)[id] = time.Now().UTC()
	b.mu.Unlock()
	b.start.Do(func() { go db.touchLoop() })
	return nil
}

// touchSession buffers a session last-seen write, or writes through when buffering is off.
func (db *DB) touchSession(sessionID, userID string, now time.Time, write func() error) error {
	b := db.touches
	if b == nil || b.interval == 0 {
		return write()
	}
	b.mu.Lock()
	b.sessions[sessionID] = sessionTouch{userID: userID, at: now}
	b.mu.Unlock()
	b.start.Do(func() { go db.touchLoop() })
	return nil
}

func (db *DB) touchLoop() {
	b := db.touches
	defer close(b.done)
	t := time.NewTicker(b.interval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			_ = db.FlushTouches(context.Background())
		case <-b.stop:
			return
		}
	}
}

// FlushTouches writes every pending touch in one transaction; failures are dropped because the columns are advisory.
func (db *DB) FlushTouches(ctx context.Context) error {
	b := db.touches
	if b == nil {
		return nil
	}
	b.mu.Lock()
	tokens, nodes, sessions := b.tokens, b.nodes, b.sessions
	b.tokens, b.nodes, b.sessions = map[string]time.Time{}, map[string]time.Time{}, map[string]sessionTouch{}
	b.mu.Unlock()
	if len(tokens)+len(nodes)+len(sessions) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for id, at := range tokens {
		if _, err := tx.ExecContext(ctx, `UPDATE api_tokens SET last_used_at = ? WHERE id = ?`, at.Format(touchTimeLayout), id); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	for id, at := range nodes {
		if _, err := tx.ExecContext(ctx, `UPDATE nodes SET last_seen_at = ? WHERE id = ?`, at.Format(touchTimeLayout), id); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	for id, t := range sessions {
		// A session removed before the flush must not abort the batch.
		_, _ = tx.ExecContext(ctx, sessionTouchSQL, id, t.userID, t.at.Unix())
	}
	return tx.Commit()
}

// closeTouches stops the flusher if it ran and drains what is pending.
func (db *DB) closeTouches() {
	b := db.touches
	if b == nil {
		return
	}
	b.stopOnce.Do(func() { close(b.stop) })
	b.start.Do(func() { close(b.done) })
	<-b.done
	_ = db.FlushTouches(context.Background())
}
