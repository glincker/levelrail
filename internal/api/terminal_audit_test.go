package api

import (
	"context"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/GLINCKER/levelrail/internal/store"
)

// TestHandleAppTerminal_RecordsAudit proves opening a shell lands in the
// audit log. It is a root-tier action, and the audit hook runs after the
// handler returns, which for a terminal means after the session ends, on
// a request context a hijacked connection may already have finished
// with: worth an explicit test rather than an assumption.
func TestHandleAppTerminal_RecordsAudit(t *testing.T) {
	fake := newFakeTTYAppRuntime()
	rt, cookie := newTerminalTestRouter(t, fake)
	conn := dialTestTerminal(t, rt, cookie, "")

	select {
	case <-fake.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the terminal session was never opened against the runtime")
	}
	_ = conn.Close(websocket.StatusNormalClosure, "done")

	waitForCondition(t, func() bool {
		entries, err := rt.auditLog.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{})
		if err != nil {
			return false
		}
		for _, entry := range entries {
			if entry.Path == "/api/v1/apps/web/terminal" && entry.Ability == AbilityRoot {
				return true
			}
		}
		return false
	}, "the terminal session to be recorded in the audit log")
}
