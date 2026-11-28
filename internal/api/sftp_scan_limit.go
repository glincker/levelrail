package api

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/glincker/levelrail/internal/runtimesettings"
)

// sftpScanFailed is the single answer to a host key scan that did not produce a key.
const sftpScanFailed = "could not read a host key from that server"

// sftpScanLimiter remembers recent scan times per caller, so one caller cannot use the scan
// as a port scanner. Entries older than a minute are dropped on each call.
var sftpScanLimiter = struct {
	mu   sync.Mutex
	seen map[string][]time.Time
}{seen: map[string][]time.Time{}}

func (rt *Router) allowSFTPScan(r *http.Request) bool {
	limit := 10
	if rt.runtimeSettings != nil {
		limit = rt.runtimeSettings.Limit(context.Background(), runtimesettings.KeyBackupSFTPScanPerMinute)
	}
	a := rt.accessActor(r)
	key := fmt.Sprintf("%p|%s:%s", rt, a.kind, a.id)
	now := time.Now()
	sftpScanLimiter.mu.Lock()
	defer sftpScanLimiter.mu.Unlock()
	recent := sftpScanLimiter.seen[key][:0]
	for _, t := range sftpScanLimiter.seen[key] {
		if now.Sub(t) < time.Minute {
			recent = append(recent, t)
		}
	}
	if len(recent) >= limit {
		sftpScanLimiter.seen[key] = recent
		return false
	}
	sftpScanLimiter.seen[key] = append(recent, now)
	return true
}
