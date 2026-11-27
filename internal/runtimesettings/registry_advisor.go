package runtimesettings

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ParseNodePrices reads "node=amount,node=amount"; amounts must be positive and finite.
func ParseNodePrices(v string) (map[string]float64, error) {
	out := map[string]float64{}
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, amt, ok := strings.Cut(part, "=")
		id = strings.TrimSpace(id)
		f, err := strconv.ParseFloat(strings.TrimSpace(amt), 64)
		if !ok || id == "" || err != nil || f <= 0 || math.IsInf(f, 0) || math.IsNaN(f) {
			return nil, fmt.Errorf("%q must look like node=amount with a positive amount", part)
		}
		out[id] = f
	}
	return out, nil
}

// FormatNodePrices is the stable text form of ParseNodePrices.
func FormatNodePrices(m map[string]float64) string {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, id+"="+strconv.FormatFloat(m[id], 'f', -1, 64))
	}
	return strings.Join(parts, ",")
}

// Registered keys for GET /api/v1/databases/{name}/advisor.
const (
	KeyAdvisorTimeoutSeconds   = "databases.advisor_timeout_seconds"
	KeyAdvisorMaxFindings      = "databases.advisor_max_findings"
	KeyAdvisorUnusedIndexBytes = "databases.advisor_unused_index_min_bytes"
	KeyAdvisorSeqScanMinRows   = "databases.advisor_seq_scan_min_rows"
	KeyAdvisorSeqScanPercent   = "databases.advisor_seq_scan_ratio"
	KeyAdvisorDeadTupleRatio   = "databases.advisor_dead_tuple_ratio"
	KeyAdvisorCacheHitMinRatio = "databases.advisor_cache_hit_min_ratio"
	KeyAdvisorIdleInTxMinutes  = "databases.advisor_idle_in_tx_minutes"
	KeyAdvisorSlowStatementMs  = "databases.advisor_slow_statement_ms"
	KeyAdvisorCacheSeconds     = "databases.advisor_cache_seconds"
	KeyAdvisorAutoApply        = "advisor.auto_apply"
	KeyAdvisorIdleHours        = "advisor.idle_hours"
	KeyAdvisorNodePrices       = "advisor.node_prices"
	advisorLiveNote            = " The next advisor run uses a changed value."
	advisorBytesCeil           = 1 << 50
)

func init() {
	Register(Definition{
		Key: KeyAdvisorAutoApply, Category: CategoryDeploys, Kind: KindBool, Default: "false",
		Env: "APP_ADVISOR_AUTO_APPLY", Apply: ApplyLive,
		Title:       "Resource advisor: auto apply",
		Description: "When on, right-sizing limits may be applied without a manual confirm. Off by default: the advisor only suggests and sends a weekly dry-run digest.",
	})
	registerLimit(Definition{Key: KeyAdvisorIdleHours, Category: CategoryDeploys, Default: "168", Env: "APP_ADVISOR_IDLE_HOURS", Apply: ApplyLive,
		Title:       "Resource advisor: idle window (hours)",
		Description: "How long an app must see no requests before the advisor suggests sleeping it. From 1 to 8760." + advisorLiveNote}, 1, 8760)
	Register(Definition{
		Key: KeyAdvisorNodePrices, Category: CategoryDeploys, Kind: KindString, Default: "", Env: "APP_ADVISOR_NODE_PRICES", Apply: ApplyLive,
		CheckValue:  func(v string) error { _, err := ParseNodePrices(v); return err },
		Title:       "Resource advisor: node monthly prices",
		Description: "Comma separated node=amount pairs, such as local=12.50,eu-1=24. Turns reclaimable memory into a monthly saving. A node without a price shows no money figure." + advisorLiveNote,
	})
	def := func(key, env, dflt, title, desc string, lo, hi int) {
		registerLimit(Definition{Key: key, Category: CategoryDeploys, Default: dflt, Env: env, Apply: ApplyLive,
			Title: title, Description: desc + advisorLiveNote}, lo, hi)
	}
	def(KeyAdvisorTimeoutSeconds, "APP_DATABASES_ADVISOR_TIMEOUT_SECONDS", "15", "Performance advisor timeout (seconds)",
		"Longest one advisor check may run against a database before it is stopped. From 1 to 300.", 1, 300)
	def(KeyAdvisorMaxFindings, "APP_DATABASES_ADVISOR_MAX_FINDINGS", "200", "Performance advisor finding cap",
		"Most findings one advisor run returns; the result says when more existed. From 1 to 2000.", 1, 2000)
	def(KeyAdvisorUnusedIndexBytes, "APP_DATABASES_ADVISOR_UNUSED_INDEX_MIN_BYTES", "8388608", "Advisor: unused index minimum size (bytes)",
		"Never-scanned indexes smaller than this are not reported. From 0 to 1 PiB.", 0, advisorBytesCeil)
	def(KeyAdvisorSeqScanMinRows, "APP_DATABASES_ADVISOR_SEQ_SCAN_MIN_ROWS", "10000", "Advisor: table size floor (rows)",
		"Tables with fewer rows than this are skipped by the sequential scan and dead tuple checks, and a foreign key on a table this large is a warning. From 0 to 1 billion.", 0, 1_000_000_000)
	def(KeyAdvisorSeqScanPercent, "APP_DATABASES_ADVISOR_SEQ_SCAN_RATIO", "50", "Advisor: sequential scan share (percent)",
		"A large table is reported when sequential scans are at least this percent of its scans. From 1 to 100.", 1, 100)
	def(KeyAdvisorDeadTupleRatio, "APP_DATABASES_ADVISOR_DEAD_TUPLE_RATIO", "20", "Advisor: dead tuple share (percent)",
		"A table is reported when dead tuples are at least this percent of its live rows. From 1 to 1000.", 1, 1000)
	def(KeyAdvisorCacheHitMinRatio, "APP_DATABASES_ADVISOR_CACHE_HIT_MIN_RATIO", "95", "Advisor: minimum cache hit (percent)",
		"The database is reported when its buffer cache hit ratio is below this percent. From 1 to 100.", 1, 100)
	def(KeyAdvisorIdleInTxMinutes, "APP_DATABASES_ADVISOR_IDLE_IN_TX_MINUTES", "5", "Advisor: long transaction (minutes)",
		"Transactions open at least this long are reported. From 1 to 10080.", 1, 10080)
	def(KeyAdvisorSlowStatementMs, "APP_DATABASES_ADVISOR_SLOW_STATEMENT_MS", "500", "Advisor: slow statement (milliseconds)",
		"Statements with a mean run time at least this long are reported when pg_stat_statements is enabled. From 1 to 3600000.", 1, 3_600_000)
	def(KeyAdvisorCacheSeconds, "APP_DATABASES_ADVISOR_CACHE_SECONDS", "60", "Advisor: result cache (seconds)",
		"How long one database's advisor result is reused before the database is asked again; 0 turns the cache off. From 0 to 3600.", 0, 3600)
}

// AdvisorSettings are the advisor's live values, read fresh on every request.
type AdvisorSettings struct {
	Timeout             time.Duration
	MaxFindings         int
	UnusedIndexMinBytes int64
	SeqScanMinRows      int64
	SeqScanPercent      int
	DeadTuplePercent    int
	CacheHitMinPercent  int
	IdleInTxMinutes     int
	SlowStatementMs     int
	CacheTTL            time.Duration
}

// Advisor reads every advisor setting.
func (s *Service) Advisor(ctx context.Context) AdvisorSettings {
	return AdvisorSettings{
		Timeout:             time.Duration(s.Limit(ctx, KeyAdvisorTimeoutSeconds)) * time.Second,
		MaxFindings:         s.Limit(ctx, KeyAdvisorMaxFindings),
		UnusedIndexMinBytes: int64(s.Int(ctx, KeyAdvisorUnusedIndexBytes)),
		SeqScanMinRows:      int64(s.Int(ctx, KeyAdvisorSeqScanMinRows)),
		SeqScanPercent:      s.Limit(ctx, KeyAdvisorSeqScanPercent),
		DeadTuplePercent:    s.Limit(ctx, KeyAdvisorDeadTupleRatio),
		CacheHitMinPercent:  s.Limit(ctx, KeyAdvisorCacheHitMinRatio),
		IdleInTxMinutes:     s.Limit(ctx, KeyAdvisorIdleInTxMinutes),
		SlowStatementMs:     s.Limit(ctx, KeyAdvisorSlowStatementMs),
		CacheTTL:            time.Duration(max(0, s.Int(ctx, KeyAdvisorCacheSeconds))) * time.Second,
	}
}
