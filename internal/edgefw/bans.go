package edgefw

import (
	"net/netip"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Ban is one automatic temporary block ("persistent action"). Bans live in
// memory only and are gone after a restart.
type Ban struct {
	IP      netip.Addr `json:"ip"`
	Reason  string     `json:"reason"`
	Started time.Time  `json:"started"`
	Ends    time.Time  `json:"ends"`
	Hits    uint64     `json:"hits"`
}

type strike struct {
	start time.Time
	n     int
}

// Bans tracks denials per client and bans a client that crosses the
// threshold. Both tables are capped.
type Bans struct {
	mu      sync.Mutex
	lim     Limits
	strikes map[netip.Addr]strike
	bans    map[netip.Addr]*Ban
	// live mirrors len(bans) so the common no-ban case never takes the lock.
	live atomic.Int64
}

// NewBans builds an empty tracker.
func NewBans(l Limits) *Bans {
	return &Bans{lim: l, strikes: make(map[netip.Addr]strike), bans: make(map[netip.Addr]*Ban)}
}

// SetLimits replaces the thresholds; existing bans keep their end times.
func (b *Bans) SetLimits(l Limits) {
	b.mu.Lock()
	b.lim = l
	b.mu.Unlock()
}

// Strike counts one denial for ip and reports whether it just became banned.
func (b *Bans) Strike(ip netip.Addr, reason string, now time.Time) bool {
	if !ip.IsValid() {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, banned := b.bans[ip]; banned {
		return false
	}
	s, ok := b.strikes[ip]
	if !ok || now.Sub(s.start) >= b.lim.BanWindow {
		if !ok && len(b.strikes) >= b.lim.MaxBans {
			b.sweepStrikes(now)
		}
		s = strike{start: now}
	}
	s.n++
	if s.n < b.lim.BanStrikes {
		b.strikes[ip] = s
		return false
	}
	delete(b.strikes, ip)
	b.addLocked(ip, reason, b.lim.BanDuration, now)
	return true
}

// Add bans ip for d.
func (b *Bans) Add(ip netip.Addr, reason string, d time.Duration, now time.Time) {
	if !ip.IsValid() {
		return
	}
	b.mu.Lock()
	b.addLocked(ip, reason, d, now)
	b.mu.Unlock()
}

func (b *Bans) addLocked(ip netip.Addr, reason string, d time.Duration, now time.Time) {
	if _, ok := b.bans[ip]; !ok && len(b.bans) >= b.lim.MaxBans {
		var oldest netip.Addr
		var oldestEnd time.Time
		for k, v := range b.bans {
			if oldestEnd.IsZero() || v.Ends.Before(oldestEnd) {
				oldest, oldestEnd = k, v.Ends
			}
		}
		delete(b.bans, oldest)
	}
	b.bans[ip] = &Ban{IP: ip, Reason: reason, Started: now, Ends: now.Add(d)}
	b.live.Store(int64(len(b.bans)))
}

func (b *Bans) sweepStrikes(now time.Time) {
	for k, s := range b.strikes {
		if now.Sub(s.start) >= b.lim.BanWindow {
			delete(b.strikes, k)
		}
	}
	for k := range b.strikes {
		if len(b.strikes) < b.lim.MaxBans {
			return
		}
		delete(b.strikes, k)
	}
}

// Active reports whether ip is banned at now, counting the hit.
func (b *Bans) Active(ip netip.Addr, now time.Time) bool {
	if b.live.Load() == 0 {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	ban, ok := b.bans[ip]
	if !ok {
		return false
	}
	if !now.Before(ban.Ends) {
		delete(b.bans, ip)
		b.live.Store(int64(len(b.bans)))
		return false
	}
	ban.Hits++
	return true
}

// List returns the unexpired bans, newest first.
func (b *Bans) List(now time.Time) []Ban {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Ban, 0, len(b.bans))
	for k, v := range b.bans {
		if !now.Before(v.Ends) {
			delete(b.bans, k)
			continue
		}
		out = append(out, *v)
	}
	b.live.Store(int64(len(b.bans)))
	sort.Slice(out, func(i, j int) bool { return out[i].Started.After(out[j].Started) })
	return out
}

// Lift removes the ban on ip and reports whether one existed.
func (b *Bans) Lift(ip netip.Addr) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.bans[ip]
	delete(b.bans, ip)
	delete(b.strikes, ip)
	b.live.Store(int64(len(b.bans)))
	return ok
}
