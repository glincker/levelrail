package plugin

import (
	"context"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/plugin/egress"
)

// denialsCache keeps the last good read so list calls do not hit the sidecar each time.
type denialsCache struct {
	mu   sync.Mutex
	at   time.Time
	byID map[string][]egress.DenialCount
}

// EgressDenials returns the sidecar's refusal counts grouped by plugin name.
// It is empty in open mode or when the sidecar cannot be read; it never errors.
func (m *Manager) EgressDenials(ctx context.Context) map[string][]egress.DenialCount {
	if !m.proxyMode() {
		return nil
	}
	m.denials.mu.Lock()
	defer m.denials.mu.Unlock()
	if m.denials.byID != nil && m.now().Sub(m.denials.at) < m.cfg.DenialsTTL {
		return m.denials.byID
	}
	rows, err := m.denialsFn(ctx, m.egressSocket(), m.cfg.DenialsTimeout)
	// Failed reads are cached too so a down sidecar costs one timeout per TTL.
	m.denials.at = m.now()
	if err != nil {
		m.log.Debug("plugin egress denials unavailable", "error", err)
		m.denials.byID = map[string][]egress.DenialCount{}
		return m.denials.byID
	}
	by := make(map[string][]egress.DenialCount, len(rows))
	for _, r := range rows {
		by[r.Plugin] = append(by[r.Plugin], r)
	}
	m.denials.byID = by
	return by
}
