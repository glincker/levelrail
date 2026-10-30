package monitor

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Defaults apply only when the matching APP_MONITOR_* variable is unset.
const (
	DefaultWorkers         = 4
	DefaultConfirmFailures = 2
	DefaultRecoveryOKs     = 2
	DefaultMinIntervalHTTP = 30 * time.Second
	DefaultMinIntervalTCP  = 30 * time.Second
	DefaultMaxTimeout      = 30 * time.Second
	DefaultMaxCount        = 100
	DefaultBodyMaxBytes    = 64 * 1024
	DefaultMaxHeaders      = 16
	DefaultHeaderMaxBytes  = 4096
	DefaultMaxRedirects    = 5
	DefaultFlushInterval   = 5 * time.Second
	DefaultBufferMax       = 10000
	DefaultRawRetention    = 48 * time.Hour
	DefaultHourRetention   = 400 * 24 * time.Hour
	DefaultPruneInterval   = 10 * time.Minute
	DefaultPruneBatch      = 1000
	DefaultJitterPercent   = 10
	DefaultTickInterval    = time.Second
	DefaultResultsMaxLimit = 1000
)

// Config holds every limit of the monitor engine, all read from APP_MONITOR* variables.
type Config struct {
	Enabled         bool
	Workers         int
	MinIntervalHTTP time.Duration
	MinIntervalTCP  time.Duration
	MaxTimeout      time.Duration
	MaxCount        int
	BodyMaxBytes    int
	MaxHeaders      int
	HeaderMaxBytes  int
	MaxRedirects    int
	FlushInterval   time.Duration
	BufferMax       int
	RawRetention    time.Duration
	HourRetention   time.Duration
	PruneInterval   time.Duration
	PruneBatch      int
	JitterPercent   int
	TickInterval    time.Duration
	ResultsMaxLimit int
	ConfirmFailures int
	RecoveryOKs     int
	UserAgent       string
}

type envReader struct {
	lookup func(string) (string, bool)
	errs   []string
}

func (e *envReader) raw(name string) (string, bool) {
	v, ok := e.lookup(name)
	v = strings.TrimSpace(v)
	return v, ok && v != ""
}

func (e *envReader) dur(name string, def time.Duration) time.Duration {
	v, ok := e.raw(name)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		e.errs = append(e.errs, name+" must be a positive duration such as 30s")
		return def
	}
	return d
}

func (e *envReader) num(name string, def, minimum int) int {
	v, ok := e.raw(name)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < minimum {
		e.errs = append(e.errs, fmt.Sprintf("%s must be an integer of at least %d", name, minimum))
		return def
	}
	return n
}

// ConfigFromEnv reads the engine settings. Unset variables take the defaults above.
func ConfigFromEnv(lookup func(string) (string, bool)) (Config, error) {
	e := &envReader{lookup: lookup}
	cfg := Config{
		Workers:         e.num("APP_MONITOR_WORKERS", DefaultWorkers, 1),
		MinIntervalHTTP: e.dur("APP_MONITOR_MIN_INTERVAL_HTTP", DefaultMinIntervalHTTP),
		MinIntervalTCP:  e.dur("APP_MONITOR_MIN_INTERVAL_TCP", DefaultMinIntervalTCP),
		MaxTimeout:      e.dur("APP_MONITOR_MAX_TIMEOUT", DefaultMaxTimeout),
		MaxCount:        e.num("APP_MONITOR_MAX_COUNT", DefaultMaxCount, 1),
		BodyMaxBytes:    e.num("APP_MONITOR_BODY_MAX_BYTES", DefaultBodyMaxBytes, 1),
		MaxHeaders:      e.num("APP_MONITOR_HEADER_MAX_COUNT", DefaultMaxHeaders, 0),
		HeaderMaxBytes:  e.num("APP_MONITOR_HEADER_MAX_BYTES", DefaultHeaderMaxBytes, 1),
		MaxRedirects:    e.num("APP_MONITOR_MAX_REDIRECTS", DefaultMaxRedirects, 0),
		FlushInterval:   e.dur("APP_MONITOR_FLUSH_INTERVAL", DefaultFlushInterval),
		BufferMax:       e.num("APP_MONITOR_BUFFER_MAX", DefaultBufferMax, 1),
		RawRetention:    e.dur("APP_MONITOR_RAW_RETENTION", DefaultRawRetention),
		HourRetention:   e.dur("APP_MONITOR_HOUR_RETENTION", DefaultHourRetention),
		PruneInterval:   e.dur("APP_MONITOR_PRUNE_INTERVAL", DefaultPruneInterval),
		PruneBatch:      e.num("APP_MONITOR_PRUNE_BATCH", DefaultPruneBatch, 1),
		JitterPercent:   e.num("APP_MONITOR_JITTER_PERCENT", DefaultJitterPercent, 0),
		TickInterval:    e.dur("APP_MONITOR_TICK_INTERVAL", DefaultTickInterval),
		ResultsMaxLimit: e.num("APP_MONITOR_RESULTS_MAX_LIMIT", DefaultResultsMaxLimit, 1),
		ConfirmFailures: e.num("APP_MONITOR_CONFIRM_FAILURES", DefaultConfirmFailures, 1),
		RecoveryOKs:     e.num("APP_MONITOR_RECOVERY_SUCCESSES", DefaultRecoveryOKs, 1),
		UserAgent:       "monitor",
	}
	if v, ok := e.raw("APP_MONITORS_ENABLED"); ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			e.errs = append(e.errs, "APP_MONITORS_ENABLED must be true or false")
		}
		cfg.Enabled = b
	}
	if cfg.JitterPercent > 100 {
		e.errs = append(e.errs, "APP_MONITOR_JITTER_PERCENT must be at most 100")
	}
	if len(e.errs) > 0 {
		return cfg, fmt.Errorf("monitor config: %s", strings.Join(e.errs, "; "))
	}
	return cfg, nil
}

// MinInterval is the per-kind floor, or zero for an unknown kind.
func (c Config) MinInterval(kind string) time.Duration {
	switch kind {
	case KindHTTP:
		return c.MinIntervalHTTP
	case KindTCP:
		return c.MinIntervalTCP
	}
	return 0
}
