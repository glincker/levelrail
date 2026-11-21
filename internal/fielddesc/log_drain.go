package fielddesc

// LogDrain is the resource name of an app log drain.
const LogDrain = "log-drains"

func init() {
	id := func(n string) (string, string) {
		return "integrations:fields.logDrain." + n + ".label", "integrations:fields.logDrain." + n + ".help"
	}
	http := []Condition{{Field: "type", Equals: []string{"http"}}}
	syslog := []Condition{{Field: "type", Equals: []string{"syslog"}}}
	tl, th := id("type")
	hl, hh := id("target_http")
	sl, sh := id("target_syslog")
	nl, nh := id("header_name")
	vl, vh := id("header_value")
	el, eh := id("enabled")
	Register(Resource{Name: LogDrain, Docs: Docs{File: "log-drain-fields.md", Title: "Log drain fields", Description: "Every field of an app log drain with its type, requirement, visibility rule and CLI flag."}, Fields: []Field{
		{ID: "type", Key: "type", Type: TypeSelect, LabelKey: tl, HelpKey: th, Default: "http", Options: []string{"http", "syslog"}, Required: true, Flag: "type", Doc: "sink type, http or syslog"},
		{ID: "target_http", Key: "target", Type: TypeString, LabelKey: hl, HelpKey: hh, Required: true, ShowWhen: http, Flag: "target", Doc: "HTTP URL that receives the log stream"},
		{ID: "target_syslog", Key: "target", Type: TypeString, LabelKey: sl, HelpKey: sh, ShowWhen: syslog, Doc: "syslog network://host:port, empty for local syslog"},
		{ID: "header_name", Key: "header_name", Type: TypeString, LabelKey: nl, HelpKey: nh, ShowWhen: http, Flag: "header-name", Doc: "optional HTTP header name sent with every batch, e.g. Authorization"},
		{ID: "header_value", Key: "header_value", Type: TypeSecret, LabelKey: vl, HelpKey: vh, Secret: true, ShowWhen: http, Flag: "header-value-file", FlagFile: true, Doc: "file holding the header value, stored encrypted and never shown again; blank keeps the stored value"},
		{ID: "enabled", Key: "enabled", Type: TypeBool, LabelKey: el, HelpKey: eh, Default: "true", FormOnly: true, Doc: "forwarding on or paused (CLI: --disabled pauses it)"},
	}})
}
