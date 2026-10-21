package migrateagent

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Audit action names, one per session event. The session id is the audit row's actor id.
const (
	ActionTokenIssued      = "migration_agent.token_issued"
	ActionEnrolled         = "migration_agent.enrolled"
	ActionEnrollRefused    = "migration_agent.enroll_refused"
	ActionSessionStarted   = "migration_agent.session_started"
	ActionSessionRejected  = "migration_agent.session_rejected"
	ActionSessionEnded     = "migration_agent.session_ended"
	ActionSessionExpired   = "migration_agent.session_expired"
	ActionDiscovery        = "migration_agent.discovery"
	ActionScan             = "migration_agent.scan"
	ActionAgentDenied      = "migration_agent.agent_denied"
	ActionVerified         = "migration_agent.verified"
	ActionStepAdvanced     = "migration_agent.step_advanced"
	ActionStepReopened     = "migration_agent.step_reopened"
	ActionRevoked          = "migration_agent.revoked"
	ActionProtocolViolated = "migration_agent.protocol_violation"
)

// AuditSaver is the audit log surface; *store.DB satisfies it.
type AuditSaver interface {
	SaveAuditEntry(ctx context.Context, e store.AuditEntry) error
}

// Event is one audit record. Detail must never hold tokens, keys or env values.
type Event struct {
	Action    string
	SessionID string
	Actor     string
	Status    int
	Remote    string
	Detail    map[string]any
}

// Auditor writes session events to the existing audit log, so retention follows it.
type Auditor struct {
	Saver  AuditSaver
	Logger *slog.Logger
	Now    func() time.Time
}

// Record writes one event. A failed write is logged, never returned, so an audit
// outage cannot be used to keep a session alive or to break a revoke.
func (a *Auditor) Record(ctx context.Context, ev Event) {
	if a == nil || a.Saver == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		a.warn("audit id", err)
		return
	}
	now := time.Now()
	if a.Now != nil {
		now = a.Now()
	}
	status := ev.Status
	if status == 0 {
		status = 200
	}
	detail := ""
	if len(ev.Detail) > 0 {
		if raw, merr := json.Marshal(ev.Detail); merr == nil {
			detail = string(raw)
		}
	}
	err = a.Saver.SaveAuditEntry(context.WithoutCancel(ctx), store.AuditEntry{
		ID: id, ActorType: "system", ActorID: ev.SessionID, ActorName: ev.Actor, Ability: "migration_agent",
		Method: "MIGRATE", Path: "/migration/agent/sessions/" + ev.SessionID, StatusCode: status, RemoteAddr: ev.Remote,
		CreatedAt: store.FormatAuditTime(now), ClientKind: "system", Action: ev.Action, Detail: detail,
	})
	if err != nil {
		a.warn("save audit entry", err)
	}
}

func (a *Auditor) warn(msg string, err error) {
	if a.Logger != nil {
		a.Logger.Warn("migrateagent: "+msg, slog.String("error", err.Error()))
	}
}
