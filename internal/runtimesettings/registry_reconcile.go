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
