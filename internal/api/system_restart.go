package api

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

const restartTriggerDelay = 300 * time.Millisecond

// Restarter lets the HTTP layer ask the process to exit so its supervisor
// relaunches it. Supervised is false when nothing would bring it back.
type Restarter struct {
	Supervised bool
	Trigger    func()
}

type restartState struct {
	mu      sync.Mutex
	r       Restarter
	bootID  string
	pending bool
}

// SetRestarter enables POST /api/v1/system/restart.
func (rt *Router) SetRestarter(r Restarter) {
	rt.restart.mu.Lock()
	rt.restart.r = r
	rt.restart.mu.Unlock()
}

type systemRestartResource struct {
	// BootID changes on every process start, so a client that saw the old
	// value knows a new process answered, not the old one draining.
	BootID    string `json:"boot_id"`
	Supported bool   `json:"supported"`
	Pending   bool   `json:"pending"`
	Reason    string `json:"reason,omitempty"`
	Hint      string `json:"hint,omitempty"`
}

func newBootID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return time.Now().UTC().Format("20060102T150405.000000000")
	}
	return hex.EncodeToString(b)
}

func (rt *Router) restartResource() systemRestartResource {
	s := &rt.restart
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bootID == "" {
		s.bootID = newBootID()
	}
	res := systemRestartResource{BootID: s.bootID, Pending: s.pending}
	switch {
	case s.r.Trigger == nil:
		res.Reason = "restart is not available on this control plane"
	case !s.r.Supported():
		res.Reason = "no process supervisor was detected, so the control plane would not come back on its own"
	default:
		res.Supported = true
	}
	if !res.Supported {
		res.Hint = "sudo systemctl restart " + rt.brand.ShortName
	}
	return res
}

// Supported reports whether a supervisor will relaunch the process.
func (r Restarter) Supported() bool { return r.Trigger != nil && r.Supervised }

func (rt *Router) handleGetSystemRestart(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, rt.restartResource())
}

func (rt *Router) handleSystemRestart(w http.ResponseWriter, r *http.Request) {
	res := rt.restartResource()
	if !res.Supported {
		writeJSON(w, http.StatusConflict, res)
		return
	}
	s := &rt.restart
	s.mu.Lock()
	if s.pending {
		s.mu.Unlock()
		writeJSON(w, http.StatusAccepted, res)
		return
	}
	s.pending = true
	trigger := s.r.Trigger
	s.mu.Unlock()

	_, _, actor, _ := rt.currentActor(r)
	rt.logger.Warn("api: control plane restart requested", slog.String("actor", actor))
	res.Pending = true
	writeJSON(w, http.StatusAccepted, res)
	// Delay so the 202 flushes before graceful shutdown starts.
	time.AfterFunc(restartTriggerDelay, trigger)
}
