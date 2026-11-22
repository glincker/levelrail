package fielddesc

// StatusWebhook is the resource name of a status page webhook subscriber.
const StatusWebhook = "status-page-webhooks"

func init() {
	f := func(id string, req bool, flag, placeholder, doc string) Field {
		return Field{ID: id, Key: id, Type: TypeString, LabelKey: "statusPage:webhooks." + id, HelpKey: "statusPage:fields.webhook." + id + ".help", Required: req, Flag: flag, Placeholder: placeholder, Doc: doc}
	}
	events := f("events", false, "event", "", "event to send, repeatable: incident_opened, incident_updated, incident_resolved, component_changed, maintenance_scheduled, maintenance_updated, maintenance_completed; empty sends all")
	events.Multi = true
	Register(Resource{Name: StatusWebhook, Docs: Docs{File: "status-webhook-fields.md", Title: "Status page webhook fields", Description: "Every field of a status page webhook subscriber with its type, requirement and CLI flag."}, Fields: []Field{
		f("url", true, "url", "https://example.com/hooks/status", "https URL that receives the signed events"),
		f("label", false, "label", "", "short name for this subscriber"),
		events,
	}})
}
