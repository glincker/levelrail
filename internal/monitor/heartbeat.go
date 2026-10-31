package monitor

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"
)

// Heartbeat signals carried by the ping URL suffix.
const (
	PingOK    = ""
	PingStart = "start"
	PingFail  = "fail"
)

// Heartbeat failure classes stored in Result.Reason.
const (
	ReasonHeartbeatMissed = "heartbeat_missed"
	ReasonJobFailed       = "job_failed"
)

const pingTokenBytes = 32

// HeartbeatState is the persisted ping history of one heartbeat monitor, unix seconds.
type HeartbeatState struct {
	MonitorID string
	TokenHash string
	ArmedAt   int64
	LastOK    int64
	LastStart int64
	LastFail  int64
}

// HeartbeatStore persists token hashes and ping times. FindHeartbeat and GetHeartbeat return ErrNotFound.
type HeartbeatStore interface {
	SaveHeartbeatToken(ctx context.Context, monitorID, tokenHash string, armedAt int64) error
	FindHeartbeat(ctx context.Context, tokenHash string) (HeartbeatState, error)
	GetHeartbeat(ctx context.Context, monitorID string) (HeartbeatState, error)
	RecordHeartbeat(ctx context.Context, monitorID, signal string, at int64) error
}

// HashPingToken is the stored form of a ping token.
func HashPingToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func newPingToken() (string, error) {
	b := make([]byte, pingTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("monitors: mint ping token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// PingPath is the public path a job calls; it embeds the secret token.
func PingPath(token string) string { return "/api/v1/heartbeat/" + token }

// grace is the monitor's grace period, falling back to the env default.
func (c Config) grace(s Spec) time.Duration {
	if s.GraceSeconds > 0 {
		return time.Duration(s.GraceSeconds) * time.Second
	}
	return c.HeartbeatGrace
}

// Ping records a signal for the monitor that owns token. Every miss (unknown token, wrong kind,
// disabled monitor, no heartbeat store) is the same ErrNotFound after the same hash and lookup work.
func (s *Service) Ping(ctx context.Context, token, signal string) error {
	hash := HashPingToken(token)
	if s.hb == nil {
		return ErrNotFound
	}
	st, err := s.hb.FindHeartbeat(ctx, hash)
	if err != nil || subtle.ConstantTimeCompare([]byte(st.TokenHash), []byte(hash)) != 1 {
		return ErrNotFound
	}
	m, err := s.store.GetMonitor(ctx, st.MonitorID)
	if err != nil || m.Kind != KindHeartbeat || !m.Enabled {
		return ErrNotFound
	}
	now := s.now()
	if err := s.hb.RecordHeartbeat(ctx, m.ID, signal, now.Unix()); err != nil {
		return err
	}
	switch signal {
	case PingFail:
		s.batcher.Add(Result{MonitorID: m.ID, At: now.Unix(), Reason: ReasonJobFailed})
		s.tracker.Force(m, true, ReasonJobFailed, false)
	case PingOK:
		s.batcher.Add(Result{MonitorID: m.ID, At: now.Unix(), OK: true})
		s.tracker.Force(m, false, "", false)
	}
	return nil
}

// RotateToken replaces a heartbeat monitor's ping token; the old URL stops working at once.
func (s *Service) RotateToken(ctx context.Context, id string) (Monitor, error) {
	m, err := s.store.GetMonitor(ctx, id)
	if err != nil {
		return Monitor{}, err
	}
	if m.Kind != KindHeartbeat || s.hb == nil {
		return Monitor{}, fmt.Errorf("%w: only heartbeat monitors have a ping token", ErrInvalid)
	}
	token, err := newPingToken()
	if err != nil {
		return Monitor{}, err
	}
	if err := s.hb.SaveHeartbeatToken(ctx, id, HashPingToken(token), s.now().Unix()); err != nil {
		return Monitor{}, err
	}
	m.PingToken = token
	return m, nil
}

// probeHeartbeat judges the ping history: failed, or silent past interval plus grace, is down.
func (s *Service) probeHeartbeat(ctx context.Context, m Monitor) {
	now := s.now()
	r := Result{MonitorID: m.ID, At: now.Unix(), OK: true}
	st, err := s.hbState(ctx, m)
	switch {
	case err != nil:
		return
	case st.LastFail > 0 && st.LastFail >= st.LastOK:
		r.OK, r.Reason = false, ReasonJobFailed
	default:
		ref := max(st.LastOK, st.LastStart, st.ArmedAt, s.started.Unix())
		if now.Sub(time.Unix(ref, 0)) > m.Interval+s.cfg.grace(m.Spec) {
			r.OK, r.Reason = false, ReasonHeartbeatMissed
		}
	}
	s.batcher.Add(r)
	s.tracker.Force(m, !r.OK, r.Reason, false)
}

func (s *Service) hbState(ctx context.Context, m Monitor) (HeartbeatState, error) {
	if s.hb == nil {
		return HeartbeatState{}, ErrNotFound
	}
	return s.hb.GetHeartbeat(ctx, m.ID)
}
