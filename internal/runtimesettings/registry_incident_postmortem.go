package runtimesettings

import "context"

// Incident postmortem bounds. Each text field is capped on its own so one long
// section cannot crowd the incident detail view or the audit trail.
const (
	KeyIncidentPostmortemFieldMax  = "incidents.postmortem_field_max"
	KeyIncidentPostmortemActionMax = "incidents.postmortem_action_items_max"
	KeyIncidentPostmortemBodyMax   = "incidents.postmortem_body_max"
)

// PostmortemLimits bounds one postmortem write, read fresh on every request.
type PostmortemLimits struct {
	FieldMax  int
	ActionMax int
	BodyMax   int
}

func init() {
	registerLimitInt(KeyIncidentPostmortemFieldMax, "APP_INCIDENT_POSTMORTEM_FIELD_MAX", "8000", "Incident postmortem: max field length", "Longest summary, impact, root cause, resolution or single action item accepted, from 256 to 65536 bytes.", 256, 65536)
	registerLimitInt(KeyIncidentPostmortemActionMax, "APP_INCIDENT_POSTMORTEM_ACTION_ITEMS_MAX", "50", "Incident postmortem: max action items", "Action items one postmortem may list, from 1 to 500.", 1, 500)
	registerLimitInt(KeyIncidentPostmortemBodyMax, "APP_INCIDENT_POSTMORTEM_BODY_MAX", "1048576", "Incident postmortem: max request body", "Largest postmortem edit request read, from 65536 to 16777216 bytes, whatever the field limits add up to.", 65536, 16<<20)
}

// PostmortemLimits returns the live postmortem bounds; a nil service serves env then defaults.
func (s *Service) PostmortemLimits(ctx context.Context) PostmortemLimits {
	return PostmortemLimits{FieldMax: s.Int(ctx, KeyIncidentPostmortemFieldMax), ActionMax: s.Int(ctx, KeyIncidentPostmortemActionMax),
		BodyMax: s.Int(ctx, KeyIncidentPostmortemBodyMax)}
}
