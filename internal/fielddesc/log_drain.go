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
	el, eh := id("enabled")
	Register(Resource{Name: LogDrain, Fields: []Field{
		{ID: "type", Key: "type", Type: TypeSelect, LabelKey: tl, HelpKey: th, Default: "http", Options: []string{"http", "syslog"}, Required: true, Flag: "type", Doc: "sink type, http or syslog"},
		{ID: "target_http", Key: "target", Type: TypeString, LabelKey: hl, HelpKey: hh, Required: true, ShowWhen: http, Flag: "target", Doc: "HTTP URL that receives the log stream"},
		{ID: "target_syslog", Key: "target", Type: TypeString, LabelKey: sl, HelpKey: sh, ShowWhen: syslog, Doc: "syslog network://host:port, empty for local syslog"},
		{ID: "enabled", Key: "enabled", Type: TypeBool, LabelKey: el, HelpKey: eh, Default: "true", Doc: "forwarding on or paused (CLI: --disabled pauses it)"},
	}})
}
