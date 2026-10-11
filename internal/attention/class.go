package attention

import "strings"

// Class separates things that are broken or about to break from setup and
// hardening advice. Only incidents belong on the dashboard and the header
// chip; recommendations live in the notification centre.
const (
	ClassIncident       = "incident"
	ClassRecommendation = "recommendation"
)

var recommendationKinds = map[string]bool{
	KindInvites:           true,
	KindTokenUnused:       true,
	KindDBSecurityUpdates: true,
	KindDBEOL:             true,
	KindDockerGuard:       true,
	"update":              true,
	"node_agent":          true,
}

// ClassOf classifies one item. An enforce-mode guard denial and every
// other unlisted kind is an incident.
func ClassOf(kind, subject string) string {
	if kind == KindDockerGuard && subject == "denied" {
		return ClassIncident
	}
	if recommendationKinds[kind] {
		return ClassRecommendation
	}
	return ClassIncident
}

// doctorIncidentCodes are the doctor checks whose warning means something
// is broken now. Every other warning is hardening advice.
var doctorIncidentCodes = map[string]bool{
	"docker":               true,
	"database":             true,
	"disk_space":           true,
	"data_dir_writable":    true,
	"container_runtime":    true,
	"control_plane_backup": true,
	"clock_skew":           true,
}

// DoctorClass classifies a doctor check that is not ok. A failed check is
// always an incident.
func DoctorClass(code, status string) string {
	if status == "fail" || doctorIncidentCodes[code] || strings.HasPrefix(code, "port_") {
		return ClassIncident
	}
	return ClassRecommendation
}
