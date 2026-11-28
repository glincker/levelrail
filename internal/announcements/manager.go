package announcements

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
)

// Modes of updates.announcements.
const (
	ModeOff      = "off"
	ModeSecurity = "security"
	ModeAll      = "all"
)

// sigSuffix names the detached signature file next to the feed.
const sigSuffix = ".sig"

// Store is the narrow store surface the Manager needs; *store.DB satisfies it.
type Store interface {
	GetAnnouncementFeedState(ctx context.Context) (store.AnnouncementFeedState, error)
	RecordAnnouncementFetch(ctx context.Context, etag, fetchedAt, lastError string) error
	SaveAnnouncementFeed(ctx context.Context, st store.AnnouncementFeedState, rows []store.AnnouncementRow, revoked []string) error
	ListAnnouncements(ctx context.Context) ([]store.AnnouncementRow, error)
	ListRevokedAnnouncementKeys(ctx context.Context) ([]string, error)
}

// Settings reads live runtime settings; *runtimesettings.Service satisfies it.
type Settings interface {
	String(ctx context.Context, key string) string
	Bool(ctx context.Context, key string) bool
	Int(ctx context.Context, key string) int
	Duration(ctx context.Context, key string) time.Duration
	List(ctx context.Context, key string) []string
}

// Manager polls, verifies and caches the feed. Poll is safe to call on every
// scheduler tick: it decides itself whether a fetch is due.
type Manager struct {
	Store    Store
	Settings Settings
	Fetcher  Fetcher
	// DevMode allows extra trusted keys from settings; never true in a release build.
	DevMode bool
	Logger  *slog.Logger
	Now     func() time.Time

	mu       sync.Mutex
	nextDue  time.Time
	failures int
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *Manager) log() *slog.Logger {
	if m.Logger != nil {
		return m.Logger
	}
	return slog.Default()
}

// LimitsFromSettings reads the feed caps from runtime settings.
func LimitsFromSettings(ctx context.Context, s Settings) Limits {
	return Limits{
		MaxBytes:  int64(s.Int(ctx, runtimesettings.KeyAnnouncementsMaxBytes)),
		MaxItems:  s.Int(ctx, runtimesettings.KeyAnnouncementsMaxItems),
		MaxTitle:  s.Int(ctx, runtimesettings.KeyAnnouncementsMaxTitle),
		MaxBody:   s.Int(ctx, runtimesettings.KeyAnnouncementsMaxBody),
		MaxAge:    s.Duration(ctx, runtimesettings.KeyAnnouncementsMaxAge),
		ClockSkew: s.Duration(ctx, runtimesettings.KeyAnnouncementsClockSkew),
	}
}

// Mode returns the validated updates.announcements value; anything else is off.
func Mode(ctx context.Context, s Settings) string {
	switch v := s.String(ctx, runtimesettings.KeyAnnouncements); v {
	case ModeSecurity, ModeAll:
		return v
	}
	return ModeOff
}

// Poll fetches and verifies the feed when one is due. It never fetches while
// the mode is off or no trusted key exists, and offline errors are silent.
func (m *Manager) Poll(ctx context.Context) error {
	if Mode(ctx, m.Settings) == ModeOff {
		return nil
	}
	m.mu.Lock()
	due := !m.now().Before(m.nextDue)
	m.mu.Unlock()
	if !due {
		return nil
	}
	keys, ignored, err := EffectiveKeys(m.DevMode, m.Settings.List(ctx, runtimesettings.KeyAnnouncementsDevKeys))
	if ignored {
		m.log().Warn("announcements: dev keys setting ignored outside dev mode")
	}
	if err != nil || len(keys) == 0 {
		m.schedule(ctx, true)
		return nil
	}
	outcome := m.fetchAndStore(ctx, keys)
	m.schedule(ctx, outcome != nil && !errors.Is(outcome, ErrUnchanged))
	return nil
}

func (m *Manager) schedule(ctx context.Context, failed bool) {
	interval := m.Settings.Duration(ctx, runtimesettings.KeyAnnouncementsInterval)
	wait := interval
	m.mu.Lock()
	defer m.mu.Unlock()
	if failed {
		m.failures++
		wait = time.Minute << min(m.failures-1, 20)
		if wait > interval || wait <= 0 {
			wait = interval
		}
	} else {
		m.failures = 0
		if jitter := m.Settings.Int(ctx, runtimesettings.KeyAnnouncementsJitter); jitter > 0 {
			//nolint:gosec,forbidigo // jitter spreads fleet polls, not security sensitive
			wait += time.Duration(rand.Float64() * float64(interval) * float64(jitter) / 100)
		}
	}
	m.nextDue = m.now().Add(wait)
}

func (m *Manager) fetchAndStore(ctx context.Context, keys []Key) error {
	url := m.Settings.String(ctx, runtimesettings.KeyAnnouncementsURL)
	lim := LimitsFromSettings(ctx, m.Settings)
	state, err := m.Store.GetAnnouncementFeedState(ctx)
	if err != nil {
		return err
	}
	now := m.now()
	stamp := store.FormatAuditTime(now)
	fetcher := m.Fetcher
	fetcher.LoopbackHTTP = m.DevMode
	res, err := fetcher.Get(ctx, url, state.ETag, lim.MaxBytes)
	if err != nil {
		m.log().Warn("announcements: fetch failed", slog.String("error", err.Error()))
		_ = m.Store.RecordAnnouncementFetch(ctx, "", stamp, ErrorCode(err))
		return err
	}
	if res.NotModified {
		return m.Store.RecordAnnouncementFetch(ctx, "", stamp, "")
	}
	sig, err := fetcher.Get(ctx, url+sigSuffix, "", lim.MaxBytes)
	if err != nil {
		m.log().Warn("announcements: signature fetch failed", slog.String("error", err.Error()))
		_ = m.Store.RecordAnnouncementFetch(ctx, "", stamp, ErrorCode(err))
		return err
	}
	revoked, err := m.Store.ListRevokedAnnouncementKeys(ctx)
	if err != nil {
		return err
	}
	ver, err := VerifyFeed(VerifyInput{
		Data: res.Body, SigData: sig.Body, Keys: keys, Revoked: setOf(revoked),
		LastSequence: state.LastSequence, Now: now, Limits: lim,
	})
	if err != nil {
		if errors.Is(err, ErrUnchanged) {
			_ = m.Store.RecordAnnouncementFetch(ctx, res.ETag, stamp, "")
			return err
		}
		m.log().Warn("announcements: feed rejected", slog.String("reason", err.Error()))
		_ = m.Store.RecordAnnouncementFetch(ctx, "", stamp, ErrorCode(err))
		return err
	}
	rows, err := rowsOf(ver)
	if err != nil {
		return err
	}
	return m.Store.SaveAnnouncementFeed(ctx, store.AnnouncementFeedState{
		LastSequence: ver.Feed.Sequence, ETag: res.ETag, FetchedAt: stamp, VerifiedAt: stamp,
		ExpiresAt: store.FormatAuditTime(ver.Feed.ExpiresAt),
	}, rows, ver.Revoked)
}

func setOf(ids []string) map[string]bool {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

func rowsOf(v Verified) ([]store.AnnouncementRow, error) {
	rows := make([]store.AnnouncementRow, 0, len(v.Items))
	for _, it := range v.Items {
		payload, err := json.Marshal(it)
		if err != nil {
			return nil, err
		}
		rows = append(rows, store.AnnouncementRow{
			ID: it.ID, Tier: it.Tier, Kind: it.Kind, PayloadJSON: string(payload),
			IssuedAt: store.FormatAuditTime(it.IssuedAt), ExpiresAt: store.FormatAuditTime(it.ExpiresAt),
			Sequence: v.Feed.Sequence,
		})
	}
	return rows, nil
}
