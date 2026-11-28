package runtimesettings

// Idle cloud node scale-in keys. This release only reports what it would remove.
const (
	KeyNodesScaleIn            = "nodes.scale_in"
	KeyNodesScaleInIdleMinutes = "nodes.scale_in_idle_minutes"
)

func init() {
	Register(Definition{
		Key: KeyNodesScaleIn, Category: CategoryDeploys, Kind: KindBool, Default: "false", Apply: ApplyLive,
		Title:       "Idle node scale-in (dry run)",
		Description: "Off by default. On, idle cloud nodes that Levelrail created and labelled autoscale=on are reported as would_destroy events with their monthly price. Nothing is destroyed in this release.",
	})
	Register(Definition{
		Key: KeyNodesScaleInIdleMinutes, Category: CategoryDeploys, Kind: KindInt, Default: "60", Apply: ApplyLive,
		CheckValue:  intRangeCheck(1, 7*24*60),
		Title:       "Idle node scale-in: idle minutes",
		Description: "How long a node must run no apps or databases before it counts as idle, from 1 to 10080 minutes.",
	})
}
