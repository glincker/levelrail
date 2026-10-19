package dashboards

// Template is a one-click starter dashboard; the picker fills the empty target names of NeedsTarget kind.
type Template struct {
	Key         string `json:"key"`
	Doc         Doc    `json:"doc"`
	NeedsTarget string `json:"needs_target,omitempty"`
}

func w(id, typ, size string, t *Target, metric string) Widget {
	return Widget{ID: id, Type: typ, Size: size, Target: t, Metric: metric}
}

// Templates returns the built-in starters.
func Templates() []Template {
	app := func() *Target { return &Target{Kind: TargetApp} }
	return []Template{
		{
			Key:         "app-health",
			NeedsTarget: TargetApp,
			Doc: Doc{Version: Version, Name: "App health", Description: "Resource use, traffic, errors and recent deploys for one app.", TimeRange: "1h", RefreshSeconds: 30, Widgets: []Widget{
				w("cpu", WidgetMetricChart, "M", app(), "cpu_percent"),
				w("memory", WidgetMetricChart, "M", app(), "memory_usage_bytes"),
				w("latency", WidgetRequestPcts, "S", app(), ""),
				w("errors", WidgetErrorRate, "S", app(), ""),
				w("alerts", WidgetAlertStatus, "S", app(), ""),
				w("deploys", WidgetDeployTimeline, "M", app(), ""),
				w("logs", WidgetLogTail, "L", app(), ""),
			}},
		},
		{
			Key: "fleet-overview",
			Doc: Doc{Version: Version, Name: "Fleet overview", Description: "Every resource at a glance.", TimeRange: "6h", RefreshSeconds: 60, Widgets: []Widget{
				w("counts", WidgetResourceCounts, "M", nil, ""),
			}},
		},
		{
			Key:         "cost",
			NeedsTarget: TargetApp,
			Doc: Doc{Version: Version, Name: "Cost", Description: "Month to date cost by tag next to the resources driving it.", TimeRange: "24h", RefreshSeconds: 300, Widgets: []Widget{
				w("cost", WidgetCost, "M", nil, ""),
				w("cpu", WidgetMetricChart, "M", app(), "cpu_percent"),
				w("memory", WidgetMetricChart, "M", app(), "memory_usage_bytes"),
			}},
		},
	}
}
