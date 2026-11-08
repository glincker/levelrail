package spec

import "sort"

// RuntimeBounds are the live limits Validate enforces, so a client can show
// them without copying the constants.
type RuntimeBounds struct {
	TmpfsMax       int      `json:"tmpfs_max"`
	TmpfsBytesMax  int      `json:"tmpfs_bytes_max"`
	ExtraHostsMax  int      `json:"extra_hosts_max"`
	DNSMax         int      `json:"dns_max"`
	StopTimeoutMax int      `json:"stop_timeout_max_seconds"`
	PidsLimitMax   int      `json:"pids_limit_max"`
	UlimitCountMax int      `json:"ulimit_count_max"`
	UlimitBytesMax int      `json:"ulimit_bytes_max"`
	Ulimits        []string `json:"ulimits"`
	Sysctls        []string `json:"sysctls"`
}

// CurrentRuntimeBounds reads the env-tuned limits at call time.
func CurrentRuntimeBounds() RuntimeBounds {
	ul := make([]string, 0, len(ulimitKinds))
	for k := range ulimitKinds {
		ul = append(ul, k)
	}
	sort.Strings(ul)
	return RuntimeBounds{
		TmpfsMax:       envPositive(envRuntimeTmpfsMax, defaultRuntimeTmpfsMax),
		TmpfsBytesMax:  envPositive(envRuntimeTmpfsBytes, defaultRuntimeTmpfsBytes),
		ExtraHostsMax:  envPositive(envRuntimeExtraHostsMax, defaultRuntimeExtraHostsMax),
		DNSMax:         envPositive(envRuntimeDNSMax, defaultRuntimeDNSMax),
		StopTimeoutMax: min(envPositive(envRuntimeStopTimeoutMax, defaultRuntimeStopTimeoutMax), MaxStopTimeoutSeconds),
		PidsLimitMax:   envPositive(envRuntimePidsMax, defaultRuntimePidsMax),
		UlimitCountMax: envPositive(envRuntimeUlimitCount, defaultRuntimeUlimitCount),
		UlimitBytesMax: envPositive(envRuntimeUlimitBytes, defaultRuntimeUlimitBytes),
		Ulimits:        ul,
		Sysctls:        sortedKeys(safeSysctls),
	}
}
