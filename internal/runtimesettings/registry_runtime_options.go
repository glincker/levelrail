package runtimesettings

import (
	"context"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// Keys for the bounds a service's runtime options are validated against.
const (
	KeyRuntimeTmpfsMax       = "runtime.tmpfs_max"
	KeyRuntimeTmpfsMaxBytes  = "runtime.tmpfs_max_bytes"
	KeyRuntimeTmpfsDefault   = "runtime.tmpfs_default_bytes"
	KeyRuntimeShmMaxBytes    = "runtime.shm_max_bytes"
	KeyRuntimeExtraHostsMax  = "runtime.extra_hosts_max"
	KeyRuntimeDNSMax         = "runtime.dns_max"
	KeyRuntimeStopTimeoutMax = "runtime.stop_timeout_max_seconds"
	KeyRuntimePidsMax        = "runtime.pids_limit_max"
	KeyRuntimeUlimitCount    = "runtime.ulimit_count_max"
	KeyRuntimeUlimitBytes    = "runtime.ulimit_bytes_max"
)

// runtimeOptionEnvKeys maps the env seed name to its setting, for the spec package.
var runtimeOptionEnvKeys = map[string]string{}

func registerRuntimeOption(key, env, title, desc string, def, lo, hi int) {
	runtimeOptionEnvKeys[env] = key
	registerLimit(Definition{
		Key: key, Category: CategoryDeploys, Default: strconv.Itoa(def), Env: env, Apply: ApplyLive, Risk: RiskWeakensSecurity, HigherIsWeaker: true,
		Title: title, Description: desc + " A change applies to the next deploy or runtime edit; running containers keep their options until recreated.",
	}, lo, hi)
}

func init() {
	const gib = 1 << 30
	registerRuntimeOption(KeyRuntimeTmpfsMax, "APP_RUNTIME_TMPFS_MAX", "Runtime options: tmpfs mounts per service",
		"Most tmpfs mounts one service may declare, from 1 to 256.", 16, 1, 256)
	registerRuntimeOption(KeyRuntimeTmpfsMaxBytes, "APP_RUNTIME_TMPFS_MAX_BYTES", "Runtime options: tmpfs size limit",
		"Largest size in bytes of one tmpfs mount, from 1 MiB to 64 GiB.", gib, 1<<20, 64*gib)
	registerRuntimeOption(KeyRuntimeTmpfsDefault, "APP_RUNTIME_TMPFS_DEFAULT_BYTES", "Runtime options: default tmpfs size",
		"Size in bytes given to a compose tmpfs entry that has no size, capped by the tmpfs size limit. From 1 MiB to 64 GiB.", 64<<20, 1<<20, 64*gib)
	registerRuntimeOption(KeyRuntimeShmMaxBytes, "APP_RUNTIME_SHM_MAX_BYTES", "Runtime options: shared memory limit",
		"Largest shm_size in bytes one service may request, from 1 MiB to 64 GiB.", 4*gib, 1<<20, 64*gib)
	registerRuntimeOption(KeyRuntimeExtraHostsMax, "APP_RUNTIME_EXTRA_HOSTS_MAX", "Runtime options: extra hosts per service",
		"Most extra_hosts entries one service may declare, from 1 to 256.", 32, 1, 256)
	registerRuntimeOption(KeyRuntimeDNSMax, "APP_RUNTIME_DNS_MAX", "Runtime options: DNS servers per service",
		"Most DNS resolvers a container may have in total, from 1 to 16.", 3, 1, 16)
	registerRuntimeOption(KeyRuntimeStopTimeoutMax, "APP_RUNTIME_STOP_TIMEOUT_MAX_SECONDS", "Runtime options: stop timeout limit",
		"Longest stop timeout in seconds a service may ask for, from 1 to 3600.", 600, 1, spec.MaxStopTimeoutSeconds)
	registerRuntimeOption(KeyRuntimePidsMax, "APP_RUNTIME_PIDS_LIMIT_MAX", "Runtime options: process cap limit",
		"Largest pids_limit one service may request, from 1 to 4194304.", 32768, 1, 4194304)
	registerRuntimeOption(KeyRuntimeUlimitCount, "APP_RUNTIME_ULIMIT_COUNT_MAX", "Runtime options: ulimit count value limit",
		"Largest value for a counting ulimit (nofile, nproc, cpu), from 1 to 1073741824.", 4194304, 1, gib)
	registerRuntimeOption(KeyRuntimeUlimitBytes, "APP_RUNTIME_ULIMIT_BYTES_MAX", "Runtime options: ulimit byte value limit",
		"Largest value in bytes for a size ulimit (core, stack, memlock, fsize), from 1 to 1 TiB.", gib, 1, 1<<40)
}

// BindRuntimeLimits points the spec bounds at s. Only the service that handles settings
// writes may call it, since another instance would cache saved rows and miss later changes.
func BindRuntimeLimits(s *Service) {
	if s == nil || s.store == nil {
		return
	}
	spec.SetRuntimeLimitSource(func(env string) (int, bool) {
		key, ok := runtimeOptionEnvKeys[env]
		if !ok {
			return 0, false
		}
		return s.Limit(context.Background(), key), true
	})
}
