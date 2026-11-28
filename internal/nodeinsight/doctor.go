package nodeinsight

import (
	"fmt"
	"time"

	"github.com/glincker/levelrail/internal/nodepreflight"
)

// Extra check IDs the doctor adds to the preflight set.
const (
	CheckCert         = "cert"
	CheckHeartbeat    = "heartbeat"
	CheckAgentVersion = "agent_version"
	CheckSchedulable  = "schedulable"
)

// DoctorFacts is what the control plane holds about a node besides the agent's
// own preflight report.
type DoctorFacts struct {
	Online          bool
	LastSeen        *time.Time
	CertState       string
	CertDaysLeft    *int
	AgentVersion    string
	ControlPlane    string
	Schedulable     bool
	HasPreflight    bool
	LocalControlRow bool
}

// ExtraChecks judges the control plane side of a node. The agent side (docker,
// disk, clock, ports) comes from nodepreflight.Evaluate.
func ExtraChecks(f DoctorFacts, now time.Time) []nodepreflight.Check {
	if f.LocalControlRow {
		return []nodepreflight.Check{}
	}
	out := []nodepreflight.Check{heartbeatCheck(f, now), certCheck(f), versionCheck(f)}
	if !f.Schedulable {
		out = append(out, nodepreflight.Check{ID: CheckSchedulable, Status: nodepreflight.StatusWarn,
			Detail: "the node is cordoned, so no new work lands on it",
			Fix:    "Uncordon the node when it should take work again.", FixCommand: "levelrail nodes uncordon <id>"})
	}
	return out
}

func heartbeatCheck(f DoctorFacts, now time.Time) nodepreflight.Check {
	if f.LastSeen == nil {
		return nodepreflight.Check{ID: CheckHeartbeat, Status: nodepreflight.StatusUnknown, Detail: "the node has not connected yet"}
	}
	age := now.Sub(*f.LastSeen).Round(time.Second)
	if f.Online {
		return nodepreflight.Check{ID: CheckHeartbeat, Status: nodepreflight.StatusPass, Detail: fmt.Sprintf("last heartbeat %s ago", age)}
	}
	return nodepreflight.Check{ID: CheckHeartbeat, Status: nodepreflight.StatusFail, Detail: fmt.Sprintf("offline, last heartbeat %s ago", age),
		Fix: "Check that the agent process or container is running and can reach the control plane."}
}

func certCheck(f DoctorFacts) nodepreflight.Check {
	detail := "the agent certificate is " + f.CertState
	if f.CertDaysLeft != nil {
		detail = fmt.Sprintf("the agent certificate has %d days left", *f.CertDaysLeft)
	}
	switch f.CertState {
	case "ok":
		return nodepreflight.Check{ID: CheckCert, Status: nodepreflight.StatusPass, Detail: detail}
	case "expiring":
		return nodepreflight.Check{ID: CheckCert, Status: nodepreflight.StatusWarn, Detail: detail, Fix: "Renew the certificate before it expires.", FixCommand: "levelrail nodes cert renew <id>"}
	case "critical", "expired", "revoked":
		return nodepreflight.Check{ID: CheckCert, Status: nodepreflight.StatusFail, Detail: detail, Fix: "Renew or re-enroll the node.", FixCommand: "levelrail nodes cert renew <id>"}
	}
	return nodepreflight.Check{ID: CheckCert, Status: nodepreflight.StatusUnknown, Detail: detail}
}

func versionCheck(f DoctorFacts) nodepreflight.Check {
	class := Classify(f.AgentVersion, f.ControlPlane)
	switch class {
	case SkewCurrent:
		return nodepreflight.Check{ID: CheckAgentVersion, Status: nodepreflight.StatusPass, Detail: "the agent matches the control plane version " + f.ControlPlane}
	case SkewBehind:
		return nodepreflight.Check{ID: CheckAgentVersion, Status: nodepreflight.StatusWarn, Detail: fmt.Sprintf("the agent runs %s, the control plane runs %s", f.AgentVersion, f.ControlPlane),
			Fix: "Upgrade the agent to the control plane version.", FixCommand: "levelrail nodes version-skew"}
	case SkewAhead:
		return nodepreflight.Check{ID: CheckAgentVersion, Status: nodepreflight.StatusWarn, Detail: fmt.Sprintf("the agent runs %s, newer than the control plane %s", f.AgentVersion, f.ControlPlane),
			Fix: "Upgrade the control plane first."}
	}
	return nodepreflight.Check{ID: CheckAgentVersion, Status: nodepreflight.StatusUnknown, Detail: "the agent has not reported a version"}
}
