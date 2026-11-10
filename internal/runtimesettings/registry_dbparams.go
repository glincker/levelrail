package runtimesettings

import "context"

// KeyDBParamsMaxConnections caps the connection parameters a database owner may
// stage (max_connections, maxclients, maxConns). The env name only seeds the default.
const KeyDBParamsMaxConnections = "databases.params_max_connections"

// KeyDBParamsMaxMemoryMB caps memory parameters of a database that has no memory limit.
const KeyDBParamsMaxMemoryMB = "databases.params_max_memory_mb"

func init() {
	Register(Definition{
		Key: KeyDBParamsMaxConnections, Category: CategoryDeploys, Kind: KindInt, Default: "5000", Env: "APP_DB_PARAMS_MAX_CONNECTIONS", Apply: ApplyLive,
		CheckValue: intRangeCheck(10, 1_000_000),
		Title:      "Database parameters: connection ceiling",
		Description: "Highest max_connections, maxclients or maxConns a managed database may be tuned to, from 10 to 1000000. " +
			"Applies to the next staged change.",
	})
	Register(Definition{
		Key: KeyDBParamsMaxMemoryMB, Category: CategoryDeploys, Kind: KindInt, Default: "4096", Env: "APP_DB_PARAMS_MAX_MEMORY_MB", Apply: ApplyLive,
		CheckValue: intRangeCheck(64, 1_048_576),
		Title:      "Database parameters: memory ceiling",
		Description: "Highest memory size parameter, in MB, for a managed database with no memory limit, from 64 to 1048576. " +
			"A database with a memory limit is capped at 75 percent of it instead.",
	})
}

// DatabaseParamsMaxMemoryMB returns the live memory ceiling for unlimited databases.
func (s *Service) DatabaseParamsMaxMemoryMB(ctx context.Context) int {
	return s.Int(ctx, KeyDBParamsMaxMemoryMB)
}

// DatabaseParamsMaxConnections returns the live connection ceiling.
func (s *Service) DatabaseParamsMaxConnections(ctx context.Context) int {
	return s.Int(ctx, KeyDBParamsMaxConnections)
}
