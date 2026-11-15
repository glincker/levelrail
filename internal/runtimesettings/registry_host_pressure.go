package runtimesettings

import "time"

// Host pressure setting keys.
const (
	KeyHostPressureEnabled      = "host.pressure_enabled"
	KeyHostPressureShedEnabled  = "host.pressure_shed_enabled"
	KeyHostPressureInterval     = "host.pressure_interval"
	KeyHostPressureHold         = "host.pressure_hold"
	KeyHostPressureExitPercent  = "host.pressure_exit_percent"
	KeyHostPressureJitter       = "host.pressure_resume_jitter"
	KeyHostPressureCPUWarn      = "host.pressure_cpu_warn"
	KeyHostPressureCPUCritical  = "host.pressure_cpu_critical"
	KeyHostPressureMemWarn      = "host.pressure_mem_warn"
	KeyHostPressureMemCritical  = "host.pressure_mem_critical"
	KeyHostPressureFullWarn     = "host.pressure_mem_full_warn"
	KeyHostPressureFullCritical = "host.pressure_mem_full_critical"
	KeyHostPressureAvailWarn    = "host.pressure_avail_warn_percent"
	KeyHostPressureAvailCrit    = "host.pressure_avail_critical_percent"
	KeyHostPressureLoadWarn     = "host.pressure_load_warn"
	KeyHostPressureLoadCritical = "host.pressure_load_critical"
	KeyHostNoSwapRAMMB          = "host.no_swap_ram_threshold_mb"
)

const (
	hostPressureMinInterval = time.Second
	hostPressureMaxInterval = time.Minute
	hostPressureMaxHold     = time.Hour
	hostPressureMaxJitter   = 10 * time.Minute
	hostPressureMaxLoad     = 10000
	hostNoSwapMaxRAMMB      = 1 << 20
)

func registerHostPressureInt(key, env, def, title, desc string, lo, hi int) {
	Register(Definition{
		Key: key, Category: CategoryObservability, Kind: KindInt, Default: def, Env: env, Apply: ApplyLive,
		CheckValue: intRangeCheck(lo, hi), Title: title, Description: desc,
	})
}

func init() {
	Register(Definition{
		Key: KeyHostPressureEnabled, Category: CategoryObservability, Kind: KindBool, Default: "true",
		Env: "APP_HOST_PRESSURE_ENABLED", Apply: ApplyLive,
		Title:       "Host pressure reporting",
		Description: "Samples CPU and memory pressure on the host, reports it in Doctor, the attention feed and metrics, and raises a host pressure event. Off stops sampling and the shedding below.",
	})
	Register(Definition{
		Key: KeyHostPressureShedEnabled, Category: CategoryObservability, Kind: KindBool, Default: "true",
		Env: "APP_HOST_PRESSURE_SHED_ENABLED", Apply: ApplyLive,
		Title:       "Shed new builds under host pressure",
		Description: "While the host is under critical pressure, new builds wait in the queue and resume after a short random delay once it recovers; while it is only warm, one build runs at a time. Running builds, rollbacks, stops and the control plane API are never held.",
	})
	Register(Definition{
		Key: KeyHostPressureInterval, Category: CategoryObservability, Kind: KindDuration, Default: "10s",
		Env: "APP_HOST_PRESSURE_INTERVAL", Apply: ApplyLive, CheckValue: durationRangeCheck(hostPressureMinInterval, hostPressureMaxInterval),
		Title:       "Host pressure sample interval",
		Description: "How often the host is sampled, from 1s to 1m. The reads are a few small files under /proc.",
	})
	Register(Definition{
		Key: KeyHostPressureHold, Category: CategoryObservability, Kind: KindDuration, Default: "2m",
		Env: "APP_HOST_PRESSURE_HOLD", Apply: ApplyLive, CheckValue: durationRangeCheck(0, hostPressureMaxHold),
		Title:       "Host pressure hold time",
		Description: "How long the host must stay below the exit thresholds before the state steps down, so a brief lull does not release queued builds into a storm.",
	})
	registerHostPressureInt(KeyHostPressureExitPercent, "APP_HOST_PRESSURE_EXIT_PERCENT", "70", "Host pressure exit threshold",
		"A state is left only when every signal is below this percent of its enter threshold, from 10 to 100.", 10, 100)
	Register(Definition{
		Key: KeyHostPressureJitter, Category: CategoryObservability, Kind: KindDuration, Default: "30s",
		Env: "APP_HOST_PRESSURE_RESUME_JITTER", Apply: ApplyLive, CheckValue: durationRangeCheck(0, hostPressureMaxJitter),
		Title:       "Queued build resume jitter",
		Description: "Each waiting build resumes after a random delay up to this long once the host recovers, so they do not all start at once. 0 resumes them together.",
	})
	registerHostPressureInt(KeyHostPressureCPUWarn, "APP_HOST_PRESSURE_CPU_WARN", "60", "CPU pressure warn",
		"Warn when the share of time some task waited for a CPU (PSI cpu some, 10 second average) reaches this percent. 0 ignores the signal.", 0, 100)
	registerHostPressureInt(KeyHostPressureCPUCritical, "APP_HOST_PRESSURE_CPU_CRITICAL", "85", "CPU pressure critical",
		"Critical when CPU pressure reaches this percent. 0 ignores the signal.", 0, 100)
	registerHostPressureInt(KeyHostPressureMemWarn, "APP_HOST_PRESSURE_MEM_WARN", "15", "Memory pressure warn",
		"Warn when the share of time some task stalled on memory (PSI memory some, 10 second average) reaches this percent. 0 ignores the signal.", 0, 100)
	registerHostPressureInt(KeyHostPressureMemCritical, "APP_HOST_PRESSURE_MEM_CRITICAL", "30", "Memory pressure critical",
		"Critical when memory pressure reaches this percent. 0 ignores the signal.", 0, 100)
	registerHostPressureInt(KeyHostPressureFullWarn, "APP_HOST_PRESSURE_MEM_FULL_WARN", "5", "Memory stall warn",
		"Warn when the share of time all tasks stalled on memory at once (PSI memory full, 10 second average) reaches this percent. 0 ignores the signal.", 0, 100)
	registerHostPressureInt(KeyHostPressureFullCritical, "APP_HOST_PRESSURE_MEM_FULL_CRITICAL", "15", "Memory stall critical",
		"Critical when the memory stall reaches this percent. 0 ignores the signal.", 0, 100)
	registerHostPressureInt(KeyHostPressureAvailWarn, "APP_HOST_PRESSURE_AVAIL_WARN_PERCENT", "10", "Available memory warn",
		"Warn when MemAvailable falls to this percent of total memory. Used with and without PSI. 0 ignores the signal.", 0, 100)
	registerHostPressureInt(KeyHostPressureAvailCrit, "APP_HOST_PRESSURE_AVAIL_CRITICAL_PERCENT", "5", "Available memory critical",
		"Critical when MemAvailable falls to this percent of total memory. 0 ignores the signal.", 0, 100)
	registerHostPressureInt(KeyHostPressureLoadWarn, "APP_HOST_PRESSURE_LOAD_WARN", "200", "Load warn (no PSI)",
		"Used only where PSI is unavailable: warn at this one minute load per CPU, times 100 (200 is a load of 2 per CPU). 0 ignores the signal.", 0, hostPressureMaxLoad)
	registerHostPressureInt(KeyHostPressureLoadCritical, "APP_HOST_PRESSURE_LOAD_CRITICAL", "400", "Load critical (no PSI)",
		"Used only where PSI is unavailable: critical at this one minute load per CPU, times 100. 0 ignores the signal.", 0, hostPressureMaxLoad)
	registerHostPressureInt(KeyHostNoSwapRAMMB, "APP_HOST_NO_SWAP_RAM_THRESHOLD_MB", "16384", "No swap warning below RAM",
		"Doctor warns about a host with no swap configured when its RAM is below this many MiB. 0 turns the warning off.", 0, hostNoSwapMaxRAMMB)
}
