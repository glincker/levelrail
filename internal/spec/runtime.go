package spec

import (
	"fmt"
	"net/netip"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Limits come from env so an operator can tighten or widen them; the
// defaults keep one app from claiming host-sized resources.
const (
	envRuntimeTmpfsDefault   = "APP_RUNTIME_TMPFS_DEFAULT_BYTES"
	envRuntimeTmpfsMax       = "APP_RUNTIME_TMPFS_MAX"
	envRuntimeTmpfsBytes     = "APP_RUNTIME_TMPFS_MAX_BYTES"
	envRuntimeShmBytes       = "APP_RUNTIME_SHM_MAX_BYTES"
	envRuntimeExtraHostsMax  = "APP_RUNTIME_EXTRA_HOSTS_MAX"
	envRuntimeDNSMax         = "APP_RUNTIME_DNS_MAX"
	envRuntimeStopTimeoutMax = "APP_RUNTIME_STOP_TIMEOUT_MAX_SECONDS"
	envRuntimePidsMax        = "APP_RUNTIME_PIDS_LIMIT_MAX"
	envRuntimeUlimitCount    = "APP_RUNTIME_ULIMIT_COUNT_MAX"
	envRuntimeUlimitBytes    = "APP_RUNTIME_ULIMIT_BYTES_MAX"

	defaultRuntimeTmpfsMax       = 16
	defaultRuntimeTmpfsBytes     = 1 << 30
	defaultRuntimeShmBytes       = 4 << 30
	defaultRuntimeExtraHostsMax  = 32
	defaultRuntimeDNSMax         = 3
	defaultRuntimeStopTimeoutMax = 600
	defaultRuntimePidsMax        = 32768
	defaultRuntimeUlimitCount    = 4194304
	defaultRuntimeUlimitBytes    = 1 << 30
)

// MaxStopTimeoutSeconds is the hard ceiling the agent wire format also applies.
const MaxStopTimeoutSeconds = 3600

// RuntimeDNSMax is how many resolvers a container may have in total.
func RuntimeDNSMax() int { return envPositive(envRuntimeDNSMax, defaultRuntimeDNSMax) }

// RuntimeOptions is the allowlisted subset of docker run options. Anything
// that widens the container's reach into the host (cap_add, privileged,
// devices, security_opt, host namespaces, host bind mounts) is not here.
type RuntimeOptions struct {
	ShmSize     string            `yaml:"shm_size,omitempty" json:"shm_size,omitempty"`
	ReadOnly    *bool             `yaml:"read_only,omitempty" json:"read_only,omitempty"`
	Tmpfs       []string          `yaml:"tmpfs,omitempty" json:"tmpfs,omitempty"`
	Ulimits     map[string]Ulimit `yaml:"ulimits,omitempty" json:"ulimits,omitempty"`
	Init        *bool             `yaml:"init,omitempty" json:"init,omitempty"`
	StopTimeout *int              `yaml:"stop_timeout,omitempty" json:"stop_timeout,omitempty"`
	ExtraHosts  []string          `yaml:"extra_hosts,omitempty" json:"extra_hosts,omitempty"`
	DNS         []string          `yaml:"dns,omitempty" json:"dns,omitempty"`
	Sysctls     map[string]string `yaml:"sysctls,omitempty" json:"sysctls,omitempty"`
	CapDrop     []string          `yaml:"cap_drop,omitempty" json:"cap_drop,omitempty"`
	PidsLimit   int               `yaml:"pids_limit,omitempty" json:"pids_limit,omitempty"`
}

// Ulimit is a soft and hard resource limit. In YAML a bare integer sets both.
type Ulimit struct {
	Soft int64 `yaml:"soft" json:"soft"`
	Hard int64 `yaml:"hard" json:"hard"`
}

// UnmarshalYAML accepts nofile: 1024 or nofile: {soft: 1024, hard: 4096}.
func (u *Ulimit) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		var n int64
		if err := node.Decode(&n); err != nil {
			return fmt.Errorf("ulimit must be an integer or {soft, hard}")
		}
		u.Soft, u.Hard = n, n
		return nil
	}
	type plain Ulimit
	return node.Decode((*plain)(u))
}

// refusedRuntimeKeys are docker options kept out of runtime on purpose.
var refusedRuntimeKeys = map[string]bool{
	"cap_add": true, "privileged": true, "devices": true, "security_opt": true,
	"pid": true, "ipc": true, "uts": true, "network_mode": true, "userns_mode": true,
	"cgroup": true, "cgroup_parent": true, "volumes": true, "binds": true, "volumes_from": true,
}

// rejectRefusedRuntime names the first refused key under a service's runtime
// block, before the generic schema error would hide the reason.
func rejectRefusedRuntime(doc any) error {
	root, _ := doc.(map[string]any)
	services, _ := root["services"].(map[string]any)
	names := make([]string, 0, len(services))
	for n := range services {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		svc, _ := services[n].(map[string]any)
		rt, _ := svc["runtime"].(map[string]any)
		keys := make([]string, 0, len(rt))
		for k := range rt {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if refusedRuntimeKeys[k] {
				return fmt.Errorf("spec: service %q: runtime.%s is not allowed: it widens the container's access to the host; see docs/app-spec-reference.md (runtime) for the supported options", n, k)
			}
		}
	}
	return nil
}

var (
	tmpfsOptPattern  = regexp.MustCompile(`^(size=[0-9]+[kKmMgG]?|mode=[0-7]{3,4}|uid=[0-9]+|gid=[0-9]+|noexec|nosuid|nodev|exec)$`)
	hostnamePattern  = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)
	sysctlValPattern = regexp.MustCompile(`^[0-9]+( [0-9]+)*$`)
)

// safeSysctls are namespaced settings Docker applies per container and
// that cannot change another tenant's kernel state.
var safeSysctls = map[string]bool{
	"net.core.somaxconn":                  true,
	"net.ipv4.ip_local_port_range":        true,
	"net.ipv4.ip_unprivileged_port_start": true,
	"net.ipv4.ping_group_range":           true,
	"net.ipv4.tcp_fin_timeout":            true,
	"net.ipv4.tcp_keepalive_intvl":        true,
	"net.ipv4.tcp_keepalive_probes":       true,
	"net.ipv4.tcp_keepalive_time":         true,
	"net.ipv4.tcp_max_syn_backlog":        true,
	"net.ipv4.tcp_syncookies":             true,
	"net.ipv4.tcp_tw_reuse":               true,
}

// ulimitKinds maps an allowed ulimit to whether its value is bytes.
var ulimitKinds = map[string]bool{
	"nofile": false, "nproc": false, "cpu": false,
	"core": true, "stack": true, "memlock": true, "fsize": true,
}

var knownCaps = func() map[string]bool {
	m := map[string]bool{"ALL": true}
	for _, c := range strings.Fields(`AUDIT_CONTROL AUDIT_READ AUDIT_WRITE BLOCK_SUSPEND BPF CHECKPOINT_RESTORE CHOWN DAC_OVERRIDE DAC_READ_SEARCH FOWNER FSETID IPC_LOCK IPC_OWNER KILL LEASE LINUX_IMMUTABLE MAC_ADMIN MAC_OVERRIDE MKNOD NET_ADMIN NET_BIND_SERVICE NET_BROADCAST NET_RAW PERFMON SETFCAP SETGID SETPCAP SETUID SYSLOG SYS_ADMIN SYS_BOOT SYS_CHROOT SYS_MODULE SYS_NICE SYS_PACCT SYS_PTRACE SYS_RAWIO SYS_RESOURCE SYS_TIME SYS_TTY_CONFIG WAKE_ALARM`) {
		m[c] = true
	}
	return m
}()

// ParseByteSize reads 64m, 1g, 512k or a plain byte count, capped at max.
func ParseByteSize(s string, limit int64) (int64, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "k"):
		mult, s = 1<<10, s[:len(s)-1]
	case strings.HasSuffix(s, "m"):
		mult, s = 1<<20, s[:len(s)-1]
	case strings.HasSuffix(s, "g"):
		mult, s = 1<<30, s[:len(s)-1]
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 || n > limit/mult {
		return 0, fmt.Errorf("must be a positive size such as 64m, at most %d bytes", limit)
	}
	return n * mult, nil
}

// ShmSizeBytes returns the validated shm_size, or 0 when unset.
func (r *RuntimeOptions) ShmSizeBytes() int64 {
	if r == nil || r.ShmSize == "" {
		return 0
	}
	n, _ := ParseByteSize(r.ShmSize, int64(envPositive(envRuntimeShmBytes, defaultRuntimeShmBytes)))
	return n
}

// Validate checks every option; errors name the field and never echo values.
func (r *RuntimeOptions) Validate() error {
	if r == nil {
		return nil
	}
	if r.ShmSize != "" {
		if _, err := ParseByteSize(r.ShmSize, int64(envPositive(envRuntimeShmBytes, defaultRuntimeShmBytes))); err != nil {
			return fmt.Errorf("runtime.shm_size: %w", err)
		}
	}
	for _, check := range []func() error{r.validateTmpfs, r.validateUlimits, r.validateHosts, r.validateSysctls, r.validateCaps, r.validateNumbers} {
		if err := check(); err != nil {
			return err
		}
	}
	return nil
}

func (r *RuntimeOptions) validateTmpfs() error {
	if limit := envPositive(envRuntimeTmpfsMax, defaultRuntimeTmpfsMax); len(r.Tmpfs) > limit {
		return fmt.Errorf("runtime.tmpfs: at most %d mounts", limit)
	}
	maxBytes := int64(envPositive(envRuntimeTmpfsBytes, defaultRuntimeTmpfsBytes))
	seen := map[string]bool{}
	for i, entry := range r.Tmpfs {
		p, opts, _ := strings.Cut(entry, ":")
		if err := ValidateFilePath(p); err != nil || p != path.Clean(p) || p == "/" {
			return fmt.Errorf("runtime.tmpfs[%d]: path must be a clean absolute path outside /proc, /sys and /dev", i)
		}
		if seen[p] {
			return fmt.Errorf("runtime.tmpfs[%d]: duplicate mount path", i)
		}
		seen[p] = true
		sized := false
		for _, o := range strings.Split(opts, ",") {
			if o == "" && opts == "" {
				break
			}
			if !tmpfsOptPattern.MatchString(o) {
				return fmt.Errorf("runtime.tmpfs[%d]: option not allowed; use size, mode, uid, gid, exec, noexec, nosuid or nodev", i)
			}
			if v, ok := strings.CutPrefix(o, "size="); ok {
				if _, err := ParseByteSize(v, maxBytes); err != nil {
					return fmt.Errorf("runtime.tmpfs[%d]: size %w", i, err)
				}
				sized = true
			}
		}
		if !sized {
			return fmt.Errorf("runtime.tmpfs[%d]: size= is required so the mount cannot grow to half of host memory", i)
		}
	}
	return nil
}

func (r *RuntimeOptions) validateUlimits() error {
	for name, u := range r.Ulimits {
		isBytes, ok := ulimitKinds[name]
		if !ok {
			return fmt.Errorf("runtime.ulimits.%s: not allowed; use nofile, nproc, cpu, core, stack, memlock or fsize", strings.ToValidUTF8(truncate(name, 24), "?"))
		}
		limit := int64(envPositive(envRuntimeUlimitCount, defaultRuntimeUlimitCount))
		if isBytes {
			limit = int64(envPositive(envRuntimeUlimitBytes, defaultRuntimeUlimitBytes))
		}
		if u.Soft < 0 || u.Hard < 0 || u.Soft > limit || u.Hard > limit {
			return fmt.Errorf("runtime.ulimits.%s: values must be between 0 and %d", name, limit)
		}
		if u.Soft > u.Hard {
			return fmt.Errorf("runtime.ulimits.%s: soft cannot exceed hard", name)
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func (r *RuntimeOptions) validateHosts() error {
	if limit := envPositive(envRuntimeExtraHostsMax, defaultRuntimeExtraHostsMax); len(r.ExtraHosts) > limit {
		return fmt.Errorf("runtime.extra_hosts: at most %d entries", limit)
	}
	for i, h := range r.ExtraHosts {
		name, ip, ok := strings.Cut(h, ":")
		if !ok || !hostnamePattern.MatchString(name) {
			return fmt.Errorf("runtime.extra_hosts[%d]: must be hostname:ip", i)
		}
		// host-gateway would reach services bound to the host's loopback side.
		if addr, err := netip.ParseAddr(strings.Trim(ip, "[]")); err != nil || addr.Zone() != "" {
			return fmt.Errorf("runtime.extra_hosts[%d]: address must be an IP, host-gateway is not allowed", i)
		}
	}
	if limit := envPositive(envRuntimeDNSMax, defaultRuntimeDNSMax); len(r.DNS) > limit {
		return fmt.Errorf("runtime.dns: at most %d servers", limit)
	}
	for i, d := range r.DNS {
		if addr, err := netip.ParseAddr(d); err != nil || addr.Zone() != "" {
			return fmt.Errorf("runtime.dns[%d]: must be an IP address", i)
		}
	}
	return nil
}

func (r *RuntimeOptions) validateSysctls() error {
	for k, v := range r.Sysctls {
		if !safeSysctls[k] {
			return fmt.Errorf("runtime.sysctls.%s: not in the allowed list (%s)", truncate(k, 48), strings.Join(sortedKeys(safeSysctls), ", "))
		}
		if !sysctlValPattern.MatchString(v) || len(v) > 32 {
			return fmt.Errorf("runtime.sysctls.%s: value must be integers separated by single spaces", k)
		}
	}
	return nil
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (r *RuntimeOptions) validateCaps() error {
	if len(r.CapDrop) > len(knownCaps) {
		return fmt.Errorf("runtime.cap_drop: too many entries")
	}
	for i, c := range r.CapDrop {
		if !knownCaps[c] {
			return fmt.Errorf("runtime.cap_drop[%d]: unknown capability, use names such as NET_RAW or ALL without the CAP_ prefix", i)
		}
	}
	return nil
}

func (r *RuntimeOptions) validateNumbers() error {
	if limit := min(envPositive(envRuntimeStopTimeoutMax, defaultRuntimeStopTimeoutMax), MaxStopTimeoutSeconds); r.StopTimeout != nil && (*r.StopTimeout < 0 || *r.StopTimeout > limit) {
		return fmt.Errorf("runtime.stop_timeout: seconds between 0 and %d", limit)
	}
	if limit := envPositive(envRuntimePidsMax, defaultRuntimePidsMax); r.PidsLimit < 0 || r.PidsLimit > limit {
		return fmt.Errorf("runtime.pids_limit: between 1 and %d", limit)
	}
	return nil
}

// validateRuntime checks the runtime block against the rest of the service.
func (svc *Service) validateRuntime(name string) error {
	r := svc.Runtime
	if r == nil {
		return nil
	}
	if err := r.Validate(); err != nil {
		return fmt.Errorf("spec: service %q: %w", name, err)
	}
	if svc.Build.Type == BuildStatic || svc.Build.Type == BuildCompose {
		return fmt.Errorf("spec: service %q: runtime is not meaningful for build.type %q, there is no single app container", name, svc.Build.Type)
	}
	if svc.Security != nil && svc.Security.ReadOnlyRootfs != nil && r.ReadOnly != nil && *svc.Security.ReadOnlyRootfs != *r.ReadOnly {
		return fmt.Errorf("spec: service %q: runtime.read_only conflicts with security.read_only_rootfs; set only one", name)
	}
	// The egress sidecar shares the network namespace; Docker refuses net.* sysctls there.
	if svc.Egress != nil && len(r.Sysctls) > 0 {
		return fmt.Errorf("spec: service %q: runtime.sysctls cannot be combined with egress, which shares the network namespace", name)
	}
	return nil
}

// ComposeKeys renders the options under the Docker Compose service keys that
// carry them, for export. Unset options are omitted.
func (r *RuntimeOptions) ComposeKeys() map[string]any {
	out := map[string]any{}
	if r == nil {
		return out
	}
	if r.ShmSize != "" {
		out["shm_size"] = r.ShmSize
	}
	if r.ReadOnly != nil {
		out["read_only"] = *r.ReadOnly
	}
	if len(r.Tmpfs) > 0 {
		out["tmpfs"] = r.Tmpfs
	}
	if len(r.Ulimits) > 0 {
		ul := make(map[string]any, len(r.Ulimits))
		for n, u := range r.Ulimits {
			ul[n] = map[string]int64{"soft": u.Soft, "hard": u.Hard}
		}
		out["ulimits"] = ul
	}
	if r.Init != nil {
		out["init"] = *r.Init
	}
	if r.StopTimeout != nil {
		out["stop_grace_period"] = strconv.Itoa(*r.StopTimeout) + "s"
	}
	if len(r.ExtraHosts) > 0 {
		out["extra_hosts"] = r.ExtraHosts
	}
	if len(r.DNS) > 0 {
		out["dns"] = r.DNS
	}
	if len(r.Sysctls) > 0 {
		out["sysctls"] = r.Sysctls
	}
	if len(r.CapDrop) > 0 {
		out["cap_drop"] = r.CapDrop
	}
	if r.PidsLimit > 0 {
		out["pids_limit"] = r.PidsLimit
	}
	return out
}

// NormalizeCap upper-cases a capability name and strips a CAP_ prefix.
func NormalizeCap(c string) string {
	return strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(c)), "CAP_")
}

// NormalizeTmpfs gives a size-less tmpfs entry a bounded default size, since
// an unsized tmpfs may grow to half of host memory.
func NormalizeTmpfs(entry string) string {
	_, opts, _ := strings.Cut(entry, ":")
	if strings.Contains(opts, "size=") {
		return entry
	}
	size := envPositive(envRuntimeTmpfsDefault, 64<<20)
	if limit := envPositive(envRuntimeTmpfsBytes, defaultRuntimeTmpfsBytes); size > limit {
		size = limit
	}
	if opts == "" {
		return fmt.Sprintf("%s:size=%d", entry, size)
	}
	return fmt.Sprintf("%s,size=%d", entry, size)
}

// Sanitize validates each option on its own. Invalid non-hardening options are
// skipped with a warning naming them; an invalid cap_drop or read_only is a
// hard error so a hardening request never fails open.
func (r *RuntimeOptions) Sanitize() (*RuntimeOptions, []string, error) {
	if r == nil {
		return nil, nil, nil
	}
	steps := []struct {
		name string
		hard bool
		set  func(dst, src *RuntimeOptions)
	}{
		{"shm_size", false, func(d, s *RuntimeOptions) { d.ShmSize = s.ShmSize }},
		{"read_only", true, func(d, s *RuntimeOptions) { d.ReadOnly = s.ReadOnly }},
		{"tmpfs", false, func(d, s *RuntimeOptions) { d.Tmpfs = s.Tmpfs }},
		{"ulimits", false, func(d, s *RuntimeOptions) { d.Ulimits = s.Ulimits }},
		{"init", false, func(d, s *RuntimeOptions) { d.Init = s.Init }},
		{"stop_timeout", false, func(d, s *RuntimeOptions) { d.StopTimeout = s.StopTimeout }},
		{"extra_hosts", false, func(d, s *RuntimeOptions) { d.ExtraHosts = s.ExtraHosts }},
		{"dns", false, func(d, s *RuntimeOptions) { d.DNS = s.DNS }},
		{"sysctls", false, func(d, s *RuntimeOptions) { d.Sysctls = s.Sysctls }},
		{"cap_drop", true, func(d, s *RuntimeOptions) { d.CapDrop = s.CapDrop }},
		{"pids_limit", false, func(d, s *RuntimeOptions) { d.PidsLimit = s.PidsLimit }},
	}
	out := &RuntimeOptions{}
	var warns []string
	for _, st := range steps {
		probe := &RuntimeOptions{}
		st.set(probe, r)
		if len(probe.ComposeKeys()) == 0 {
			continue
		}
		if err := probe.Validate(); err != nil {
			if st.hard {
				return nil, nil, err
			}
			warns = append(warns, fmt.Sprintf("%v; %s was not applied", err, st.name))
			continue
		}
		st.set(out, r)
	}
	if len(out.ComposeKeys()) == 0 {
		return nil, warns, nil
	}
	return out, warns, nil
}
