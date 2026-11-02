package desktopmode

import (
	"sync"
	"time"
)

const maxTrackedPeers = 1024

// limiter counts bad tokens per peer address and blocks a peer for a short
// time when it passes the cap. A valid token never goes through it.
type limiter struct {
	mu    sync.Mutex
	cap   int
	block time.Duration
	now   func() time.Time
	peers map[string]*peerState
}

type peerState struct {
	fails        []time.Time
	blockedUntil time.Time
}

// fail records a bad token from peer. It returns true (with the wait) when the peer is blocked.
func (l *limiter) fail(peer string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if l.peers == nil {
		l.peers = make(map[string]*peerState)
	}
	if len(l.peers) >= maxTrackedPeers {
		for k, p := range l.peers {
			if now.After(p.blockedUntil) && (len(p.fails) == 0 || now.Sub(p.fails[len(p.fails)-1]) > time.Minute) {
				delete(l.peers, k)
			}
		}
	}
	p := l.peers[peer]
	if p == nil {
		p = &peerState{}
		l.peers[peer] = p
	}
	if now.Before(p.blockedUntil) {
		return true, p.blockedUntil.Sub(now)
	}
	kept := p.fails[:0]
	for _, t := range p.fails {
		if now.Sub(t) < time.Minute {
			kept = append(kept, t)
		}
	}
	p.fails = append(kept, now)
	if l.cap > 0 && len(p.fails) >= l.cap {
		p.blockedUntil = now.Add(l.block)
		p.fails = nil
		return true, l.block
	}
	return false, 0
}
