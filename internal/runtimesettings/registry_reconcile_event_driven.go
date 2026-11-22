package runtimesettings

// KeyReconcileEventDriven lets a reconcile pass skip apps that settled and saw no change.
const KeyReconcileEventDriven = "reconcile.event_driven"

func init() {
	Register(Definition{
		Key: KeyReconcileEventDriven, Category: CategoryObservability, Kind: KindBool, Default: "true",
		Apply:       ApplyLive,
		Title:       "Event driven reconcile",
		Description: "On by default: a pass skips apps that are Ready and saw no container event, change or probe failure since their last run, and the full resync still covers everything. Turn off to reconcile every app on every pass.",
	})
}
