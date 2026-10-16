package maintenance

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Why domain maintenance ended, recorded on the end audit row.
const (
	EndManual        = "manual"
	EndUntil         = "until"
	EndMaxDuration   = "max_duration"
	EndDomainRemoved = "domain_removed"
)

// Audit actions for the maintenance lifecycle rows.
const (
	AuditActionStarted = "domain_maintenance.started"
	AuditActionEnded   = store.AuditActionDomainMaintenanceEnded
)

// AuditDetail is the JSON detail on a maintenance lifecycle audit row.
type AuditDetail struct {
	Domain          string `json:"domain"`
	App             string `json:"app,omitempty"`
	StartedAt       string `json:"started_at,omitempty"`
	DurationSeconds int64  `json:"duration_seconds"`
	EndReason       string `json:"end_reason,omitempty"`
	Until           string `json:"until,omitempty"`
	AutoOffAt       string `json:"auto_off_at,omitempty"`
	Actor           string `json:"actor"`
}

// Actor is who caused a maintenance change.
type Actor struct {
	Type, ID, Name, ClientKind, RemoteAddr string
}

// SystemActor is the actor for automatic ends.
var SystemActor = Actor{Type: "system", ID: "maintenance-expiry", Name: "Scheduled maintenance", ClientKind: "system"}

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// EndReasonFor says which limit ended m on its own at now.
func EndReasonFor(m store.DomainMaintenance, now time.Time) string {
	if !m.AutoOffAt.IsZero() && !now.Before(m.AutoOffAt) {
		return EndMaxDuration
	}
	return EndUntil
}

// StartedDetail describes maintenance that has just begun.
func StartedDetail(m store.DomainMaintenance, app string, a Actor) AuditDetail {
	return AuditDetail{Domain: m.Domain, App: app, StartedAt: fmtTime(m.StartedAt),
		Until: fmtTime(m.EndsAt), AutoOffAt: fmtTime(m.AutoOffAt), Actor: actorLabel(a)}
}

// EndedDetail describes maintenance that has just ended for reason.
func EndedDetail(m store.DomainMaintenance, app, reason string, a Actor, now time.Time) AuditDetail {
	d := StartedDetail(m, app, a)
	d.DurationSeconds = int64(Elapsed(m.StartedAt, now).Seconds())
	d.EndReason = reason
	return d
}

func actorLabel(a Actor) string {
	if a.Name != "" {
		return a.Name
	}
	return a.ID
}

// NewAuditEntry builds the lifecycle audit row for action with detail.
func NewAuditEntry(action string, d AuditDetail, a Actor, now time.Time) (store.AuditEntry, error) {
	id, err := store.NewAuditEntryID()
	if err != nil {
		return store.AuditEntry{}, err
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return store.AuditEntry{}, err
	}
	method := http.MethodPut
	if action == AuditActionEnded {
		method = "EVENT"
	}
	return store.AuditEntry{
		ID: id, ActorType: a.Type, ActorID: a.ID, ActorName: a.Name, Ability: action, Method: method,
		Path: "/api/v1/domains/" + d.Domain + "/maintenance", StatusCode: http.StatusOK, RemoteAddr: a.RemoteAddr,
		CreatedAt: store.FormatAuditTime(now), ClientKind: a.ClientKind, Action: action, Detail: string(raw),
	}, nil
}
