package dbstats

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// psqlCommand runs inside the container and reads the root user from its own environment, so no secret crosses argv.
// The session is read only and bounded by statement and lock timeouts, a connect timeout and, when present, timeout(1).
func psqlCommand(timeoutMS int, sql string) string {
	secs := (timeoutMS+999)/1000 + 1
	return `W=$(command -v timeout || true); [ -n "$W" ] && W="$W ` + strconv.Itoa(secs) + `"; ` +
		`PGCONNECT_TIMEOUT=5 PGAPPNAME=levelrail-stats ` +
		`PGOPTIONS='-c statement_timeout=` + strconv.Itoa(timeoutMS) + ` -c lock_timeout=1000 -c default_transaction_read_only=on' ` +
		`exec $W psql -X -At -F '=' --no-password -U "$POSTGRES_USER" "$POSTGRES_USER" -c "` + sql + `"`
}

// counterMetrics are the per database counters, in the order the script prints them.
var counterMetrics = []string{
	telemetry.MetricDBBlksHit, telemetry.MetricDBBlksRead,
	telemetry.MetricDBTupInserted, telemetry.MetricDBTupUpdated, telemetry.MetricDBTupDeleted,
	telemetry.MetricDBDeadlocks, telemetry.MetricDBXactCommit, telemetry.MetricDBXactRollback,
}

func scalarSQL(maxRows int) string {
	const clients = `FROM pg_stat_activity WHERE backend_type = 'client backend' AND pid <> pg_backend_pid()`
	return `SELECT 'conn_active', (count(*) FILTER (WHERE state = 'active'))::text ` + clients +
		` UNION ALL SELECT 'conn_idle', (count(*) FILTER (WHERE state LIKE 'idle%'))::text ` + clients +
		` UNION ALL SELECT 'conn_total', count(*)::text ` + clients +
		` UNION ALL SELECT 'conn_max', current_setting('max_connections') ` +
		` UNION ALL SELECT 'locks_waiting', count(*)::text FROM pg_locks WHERE NOT granted ` +
		` UNION ALL (SELECT 'db', concat_ws('|', datid, coalesce(extract(epoch FROM stats_reset)::bigint, 0), ` +
		`blks_hit, blks_read, tup_inserted, tup_updated, tup_deleted, deadlocks, xact_commit, xact_rollback) ` +
		`FROM pg_stat_database WHERE datid <> 0 ORDER BY datid LIMIT ` + strconv.Itoa(maxRows) + `)`
}

func sizeSQL(maxRows int) string {
	return `SELECT 'size.' || datname, pg_database_size(datname)::text FROM pg_database ` +
		`WHERE NOT datistemplate AND datname ~ '^[A-Za-z0-9_.-]+$' ORDER BY pg_database_size(datname) DESC LIMIT ` + strconv.Itoa(maxRows)
}

// scalarMetrics maps a script key to its registered gauge name.
var scalarMetrics = map[string]string{
	"conn_active":   telemetry.MetricDBConnActive,
	"conn_idle":     telemetry.MetricDBConnIdle,
	"conn_total":    telemetry.MetricDBConnTotal,
	"conn_max":      telemetry.MetricDBConnMax,
	"locks_waiting": telemetry.MetricDBLocksWaiting,
}

// metricValue is one parsed reading before it gets a resource and timestamp.
type metricValue struct {
	Metric string
	Value  float64
}

// dbRow is one pg_stat_database row; Reset changes when the counters were zeroed.
type dbRow struct {
	Reset    int64
	Counters [8]float64
}

type scalarResult struct {
	Gauges []metricValue
	Rows   map[int64]dbRow
}

// parseScalars turns key=value lines into gauges and per database counter rows. Unknown keys are skipped.
func parseScalars(out string) (scalarResult, error) {
	res := scalarResult{Rows: map[int64]dbRow{}}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, raw, ok := strings.Cut(line, "=")
		if !ok {
			return res, fmt.Errorf("unexpected stats line %q", line)
		}
		if key == "db" {
			id, row, err := parseRow(raw)
			if err != nil {
				return res, err
			}
			res.Rows[id] = row
			continue
		}
		name, known := scalarMetrics[key]
		if !known {
			continue
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return res, fmt.Errorf("unexpected stats value in %q", line)
		}
		res.Gauges = append(res.Gauges, metricValue{Metric: name, Value: v})
	}
	if len(res.Gauges) == 0 {
		return res, fmt.Errorf("empty stats output")
	}
	return res, nil
}

func parseRow(raw string) (int64, dbRow, error) {
	f := strings.Split(raw, "|")
	if len(f) != 2+len(counterMetrics) {
		return 0, dbRow{}, fmt.Errorf("unexpected database row %q", raw)
	}
	id, err := strconv.ParseInt(f[0], 10, 64)
	if err != nil {
		return 0, dbRow{}, fmt.Errorf("unexpected database row %q", raw)
	}
	row := dbRow{}
	if row.Reset, err = strconv.ParseInt(f[1], 10, 64); err != nil {
		return 0, dbRow{}, fmt.Errorf("unexpected database row %q", raw)
	}
	for i := range row.Counters {
		if row.Counters[i], err = strconv.ParseFloat(f[2+i], 64); err != nil {
			return 0, dbRow{}, fmt.Errorf("unexpected database row %q", raw)
		}
	}
	return id, row, nil
}

// parseSizes reads size.<name>=<bytes> lines, keeping at most maxDatabases series.
func parseSizes(out string, maxDatabases int) ([]metricValue, error) {
	var vals []metricValue
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, raw, ok := strings.Cut(line, "=")
		name, isSize := strings.CutPrefix(key, "size.")
		if !ok || !isSize {
			return nil, fmt.Errorf("unexpected size line %q", line)
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("unexpected size value in %q", line)
		}
		if len(vals) >= maxDatabases {
			break
		}
		vals = append(vals, metricValue{Metric: telemetry.MetricDBSizePrefix + name, Value: v})
	}
	return vals, nil
}

// counterDeltas is the instance level increase between two samples. A database that vanished, appeared or was
// stats-reset contributes nothing, and a counter that went backwards counts as 0, so no tick can write a spike.
func counterDeltas(prev, cur map[int64]dbRow) []metricValue {
	sums := make([]float64, len(counterMetrics))
	for id, now := range cur {
		before, ok := prev[id]
		if !ok || before.Reset != now.Reset {
			continue
		}
		for i := range sums {
			if d := now.Counters[i] - before.Counters[i]; d > 0 {
				sums[i] += d
			}
		}
	}
	out := make([]metricValue, len(sums))
	for i, s := range sums {
		out[i] = metricValue{Metric: counterMetrics[i], Value: s}
	}
	return out
}
