package runtimesettings

// KeyReconcileResyncSeconds is the safety-net interval between full reconcile passes.
const KeyReconcileResyncSeconds = "reconcile.resync_seconds"

func init() {
	registerLimit(Definition{
		Key: KeyReconcileResyncSeconds, Category: CategoryObservability, Default: "30", Env: "APP_RECONCILE_RESYNC_SECONDS", Apply: ApplyLive,
		Title:       "Reconcile resync interval",
		Description: "Seconds between full reconcile passes that catch any event the stream missed. From 5 to 3600. Takes effect at the next resync tick.",
	}, 5, 3600)
}

// KeyReconcileLagWarnSeconds is how long a desired change may wait before a reconciler counts as lagging.
const KeyReconcileLagWarnSeconds = "reconcile.lag_warn_seconds"

func init() {
	registerLimit(Definition{
		Key: KeyReconcileLagWarnSeconds, Category: CategoryObservability, Default: "120", Env: "APP_RECONCILE_LAG_WARN_SECONDS", Apply: ApplyLive,
		Title:       "Reconciler lag warning",
		Description: "Seconds a desired change may wait for a successful reconcile before the doctor reports that reconciler as lagging. From 10 to 86400. Read on every check.",
	}, 10, 86400)
}
