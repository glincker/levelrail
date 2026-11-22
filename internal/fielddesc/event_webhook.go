package fielddesc

// EventWebhook is the resource name of an outbound event webhook.
const EventWebhook = "event-webhooks"

func init() {
	f := func(id string, req bool, flag, placeholder, doc string) Field {
		return Field{ID: id, Key: id, Type: TypeString, LabelKey: "eventWebhooks:form." + id, HelpKey: "eventWebhooks:fields." + id + ".help", Required: req, Flag: flag, Placeholder: placeholder, Doc: doc}
	}
	events := f("events", false, "event", "apps.*, users.create", "event type or family such as apps.*, comma separated or repeated; empty delivers every event")
	events.Multi = true
	Register(Resource{Name: EventWebhook, Docs: Docs{File: "event-webhook-fields.md", Title: "Event webhook fields", Description: "Every field of an outbound event webhook with its type, requirement and CLI flag."}, Fields: []Field{
		f("name", true, "name", "siem", "webhook name"),
		f("url", true, "url", "https://hooks.example.com/events", "https endpoint that receives signed POSTs"),
		events,
	}})
}
