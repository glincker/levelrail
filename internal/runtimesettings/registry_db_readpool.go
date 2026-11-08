package runtimesettings

// KeyReadConns sizes the control plane database read pool.
const (
	KeyReadConns = "store.read_connections"
	maxReadConns = 32
)

func init() {
	registerLimit(Definition{
		Key: KeyReadConns, Category: CategoryObservability, Default: "4", Env: "APP_DB_READ_CONNS", Apply: ApplyLive,
		Title:       "Database read connections",
		Description: "How many read-only connections the control plane database keeps, from 1 to 32. A saved change resizes the pool within a minute. Turning the pool off (APP_DB_READ_CONNS=0) stays a boot-only environment choice and is not changed live.",
	}, 1, maxReadConns)
}
