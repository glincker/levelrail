package cloudiac

import (
	"errors"
	"fmt"
	"net/netip"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Fixed paths inside the step container.
const (
	SandboxModuleDir     = "/module"
	SandboxWorkDir       = "/work"
	SandboxStateDir      = "/state"
	SandboxTofuPath      = "/opt/tofu/tofu"
	SandboxPluginDir     = "/opt/tofu/plugins"
	SandboxCredentialDir = "/run/lr-cred" //nolint:gosec // a mount path, not a credential
)

// ErrSandboxSpec marks a step spec the builder refuses to produce.
var ErrSandboxSpec = errors.New("cloudiac: unsafe sandbox spec")

// builtinDenyCIDRs are never reachable from a step: loopback, link-local metadata and this-network.
var builtinDenyCIDRs = []string{
	"0.0.0.0/8",
	"127.0.0.0/8",
	"169.254.0.0/16",
	"100.100.100.200/32",
	"::1/128",
	"fe80::/10",
	"fd00:ec2::254/128",
}

// NetworkPolicy is the egress rule set for step containers, applied to a pre-created network.
type NetworkPolicy struct {
	Network   string
	DenyCIDRs []string
}

// NewNetworkPolicy adds the control plane addresses to the builtin deny list.
// The network itself is created and firewalled outside this package from DenyCIDRs.
func NewNetworkPolicy(network string, controlPlane []string) (NetworkPolicy, error) {
	switch n := strings.ToLower(network); {
	case n == "", n == "host", n == "bridge", n == "default", n == "none", strings.HasPrefix(n, "container:"):
		return NetworkPolicy{}, fmt.Errorf("%w: step network must be a dedicated pre-created network", ErrSandboxSpec)
	}
	if len(controlPlane) == 0 {
		return NetworkPolicy{}, fmt.Errorf("%w: control plane addresses are required for the deny list", ErrSandboxSpec)
	}
	deny := append([]string{}, builtinDenyCIDRs...)
	for _, a := range controlPlane {
		p, err := parsePrefix(a)
		if err != nil {
			return NetworkPolicy{}, fmt.Errorf("%w: control plane address %q", ErrSandboxSpec, a)
		}
		deny = append(deny, p.String())
	}
	return NetworkPolicy{Network: network, DenyCIDRs: deny}, nil
}

func parsePrefix(s string) (netip.Prefix, error) {
	if strings.Contains(s, "/") {
		p, err := netip.ParsePrefix(s)
		return p.Masked(), err
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	return netip.PrefixFrom(a, a.BitLen()), nil
}

// RunFlags returns the container flags that attach a step to the policy network.
func (p NetworkPolicy) RunFlags() []string { return []string{"--network=" + p.Network} }

// Denies reports whether addr falls in the deny list; the firewall step and tests share it.
func (p NetworkPolicy) Denies(addr string) bool {
	a, err := netip.ParseAddr(addr)
	if err != nil {
		return true
	}
	for _, c := range p.DenyCIDRs {
		if pr, err := netip.ParsePrefix(c); err == nil && pr.Contains(a.Unmap()) {
			return true
		}
	}
	return false
}

// SandboxLimits bound one step. Values come from runtime settings; zero refuses.
// WorkTmpMB is the free space the run directory filesystem must have; StateTmpMB sizes /tmp.
type SandboxLimits struct {
	MemoryMB   int
	CPUs       float64
	Pids       int
	WorkTmpMB  int
	StateTmpMB int
}

// CredentialFile is the one short-lived credential file of a step. Only its path enters the spec.
type CredentialFile struct {
	// HostPath is a file on a host tmpfs, written for this step and removed after it.
	HostPath string
	// EnvName names the variable that tells the provider where the file is.
	EnvName string
}

// SandboxInput describes one step. Nothing in it may carry a secret value.
type SandboxInput struct {
	Runtime   string
	Image     string
	User      string
	Name      string
	Step      Step
	ModuleDir string
	// RunDir is the private per-run host directory; its work and state subdirs are shared by every step.
	RunDir     string
	TofuBinary string
	PluginDir  string
	Credential CredentialFile
	Network    NetworkPolicy
	Limits     SandboxLimits
	Env        map[string]string
}

// Mount is a bind mount, read-only unless ReadWrite is set for the run directory.
type Mount struct {
	Source    string
	Target    string
	ReadWrite bool
}

// Tmpfs is an in-memory mount private to the step.
type Tmpfs struct {
	Target  string
	Options string
}

// SandboxSpec is the typed run spec for one step container.
type SandboxSpec struct {
	Runtime      string
	Image        string
	Name         string
	User         string
	ReadOnlyRoot bool
	CapDrop      []string
	SecurityOpt  []string
	PidsLimit    int
	MemoryMB     int
	CPUs         string
	NetworkFlags []string
	Mounts       []Mount
	Tmpfs        []Tmpfs
	Env          []string
	Workdir      string
	Entrypoint   string
	Args         []string
}

var (
	imageDigestRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9._/:-]*@sha256:[a-f0-9]{64}$`)
	userRe          = regexp.MustCompile(`^[1-9][0-9]*:[1-9][0-9]*$`)
	envNameRe       = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
	containerNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,62}$`)
	secretNameRe    = regexp.MustCompile(`SECRET|TOKEN|PASSWORD|PASSPHRASE|PRIVATE|ACCESS_KEY|API_KEY|CREDENTIAL`)
)

// StepArgs returns the tofu arguments for a step. Apply only runs the saved plan, never a fresh diff.
func StepArgs(step Step) ([]string, error) {
	base := []string{"-chdir=" + SandboxModuleDir}
	plan := SandboxStateDir + "/plan.tfplan"
	switch step {
	case StepPlan:
		return append(base, "plan", "-input=false", "-no-color", "-lock=false", "-out="+plan), nil
	case StepDestroy:
		return append(base, "plan", "-destroy", "-input=false", "-no-color", "-lock=false", "-out="+plan), nil
	case StepApply:
		return append(base, "apply", "-input=false", "-no-color", "-lock=false", plan), nil
	}
	return nil, fmt.Errorf("%w: unknown step %q", ErrSandboxSpec, step)
}

// BuildSandboxSpec validates the input and returns the hardened run spec.
func BuildSandboxSpec(in SandboxInput) (SandboxSpec, error) {
	if in.Runtime != "docker" && in.Runtime != "podman" {
		return SandboxSpec{}, fmt.Errorf("%w: runtime must be docker or podman", ErrSandboxSpec)
	}
	if !imageDigestRe.MatchString(in.Image) {
		return SandboxSpec{}, fmt.Errorf("%w: image must be pinned by sha256 digest", ErrSandboxSpec)
	}
	if !userRe.MatchString(in.User) {
		return SandboxSpec{}, fmt.Errorf("%w: user must be a numeric non-root uid:gid", ErrSandboxSpec)
	}
	if in.Name != "" && !containerNameRe.MatchString(in.Name) {
		return SandboxSpec{}, fmt.Errorf("%w: container name", ErrSandboxSpec)
	}
	l := in.Limits
	if l.MemoryMB <= 0 || l.CPUs <= 0 || l.Pids <= 0 || l.WorkTmpMB <= 0 || l.StateTmpMB <= 0 {
		return SandboxSpec{}, fmt.Errorf("%w: every limit is required", ErrSandboxSpec)
	}
	if len(in.Network.DenyCIDRs) < len(builtinDenyCIDRs) || in.Network.Network == "" {
		return SandboxSpec{}, fmt.Errorf("%w: network policy must come from NewNetworkPolicy", ErrSandboxSpec)
	}
	args, err := StepArgs(in.Step)
	if err != nil {
		return SandboxSpec{}, err
	}

	mounts := []Mount{{Source: in.ModuleDir, Target: SandboxModuleDir}, {Source: in.TofuBinary, Target: SandboxTofuPath}}
	if in.PluginDir != "" {
		mounts = append(mounts, Mount{Source: in.PluginDir, Target: SandboxPluginDir})
	}
	credTarget := SandboxCredentialDir + "/credential"
	mounts = append(mounts, Mount{Source: in.Credential.HostPath, Target: credTarget})
	if err := checkRunDir(in.RunDir, in.ModuleDir, in.Credential.HostPath); err != nil {
		return SandboxSpec{}, err
	}
	mounts = append(mounts,
		Mount{Source: filepath.Join(in.RunDir, RunWorkSubdir), Target: SandboxWorkDir, ReadWrite: true},
		Mount{Source: filepath.Join(in.RunDir, RunStateSubdir), Target: SandboxStateDir, ReadWrite: true})
	for _, m := range mounts {
		if err := checkMountSource(m.Source); err != nil {
			return SandboxSpec{}, err
		}
	}
	if !envNameRe.MatchString(in.Credential.EnvName) {
		return SandboxSpec{}, fmt.Errorf("%w: credential env name", ErrSandboxSpec)
	}

	env := []string{
		"HOME=" + SandboxWorkDir,
		"TF_DATA_DIR=" + SandboxWorkDir + "/.tofu",
		"TF_IN_AUTOMATION=1",
		"TF_INPUT=0",
		in.Credential.EnvName + "=" + credTarget,
	}
	if in.PluginDir != "" {
		env = append(env, "TF_PLUGIN_CACHE_DIR="+SandboxPluginDir)
	}
	extra, err := checkedEnv(in.Env, in.Credential.EnvName)
	if err != nil {
		return SandboxSpec{}, err
	}
	env = append(env, extra...)

	uid, gid, _ := strings.Cut(in.User, ":")
	owner := ",uid=" + uid + ",gid=" + gid + ",mode=0700"
	return SandboxSpec{
		Runtime:      in.Runtime,
		Image:        in.Image,
		Name:         in.Name,
		User:         in.User,
		ReadOnlyRoot: true,
		CapDrop:      []string{"ALL"},
		SecurityOpt:  []string{"no-new-privileges"},
		PidsLimit:    l.Pids,
		MemoryMB:     l.MemoryMB,
		CPUs:         strconv.FormatFloat(l.CPUs, 'f', -1, 64),
		NetworkFlags: in.Network.RunFlags(),
		Mounts:       mounts,
		Tmpfs: []Tmpfs{
			{Target: "/tmp", Options: "rw,noexec,nosuid,nodev,size=" + strconv.Itoa(l.StateTmpMB) + "m" + owner},
		},
		Env:        env,
		Workdir:    SandboxWorkDir,
		Entrypoint: SandboxTofuPath,
		Args:       args,
	}, nil
}

// checkRunDir keeps the writable run dir apart from the read-only module and credential mounts,
// so a step can never gain write access to either through an overlapping path.
func checkRunDir(run, module, cred string) error {
	if run == "" || !filepath.IsAbs(run) || filepath.Clean(run) != run || run == "/" {
		return fmt.Errorf("%w: run dir must be a clean absolute path", ErrSandboxSpec)
	}
	for _, other := range []string{module, cred} {
		if other == "" {
			continue
		}
		if other == run || strings.HasPrefix(other, run+"/") || strings.HasPrefix(run, other+"/") {
			return fmt.Errorf("%w: run dir must not overlap the module or credential mounts", ErrSandboxSpec)
		}
	}
	return nil
}

// checkMountSource refuses relative paths, the host root and any container runtime socket.
func checkMountSource(src string) error {
	if src == "" || !filepath.IsAbs(src) || filepath.Clean(src) != src || src == "/" {
		return fmt.Errorf("%w: mount source %q must be a clean absolute path", ErrSandboxSpec, src)
	}
	base := path.Base(src)
	if strings.HasSuffix(base, ".sock") || strings.Contains(src, "docker") && strings.Contains(src, "sock") {
		return fmt.Errorf("%w: runtime sockets are never mounted", ErrSandboxSpec)
	}
	for _, p := range []string{"/proc", "/sys", "/dev", "/etc", "/var/run", "/run/docker", "/run/podman"} {
		if src == p || strings.HasPrefix(src, p+"/") {
			return fmt.Errorf("%w: host path %q is never mounted", ErrSandboxSpec, src)
		}
	}
	return nil
}

func checkedEnv(in map[string]string, credEnv string) ([]string, error) {
	out := make([]string, 0, len(in))
	for k, v := range in {
		if !envNameRe.MatchString(k) || k == credEnv || strings.HasPrefix(k, "TF_") {
			return nil, fmt.Errorf("%w: env name %q", ErrSandboxSpec, k)
		}
		// Secrets never travel as env or argv; only the credential file path does.
		if secretNameRe.MatchString(k) {
			return nil, fmt.Errorf("%w: env %q looks like a secret, use the credential file", ErrSandboxSpec, k)
		}
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out, nil
}

// Argv returns the full container command line, starting with the runtime binary.
func (s SandboxSpec) Argv() []string {
	argv := []string{s.Runtime, "run", "--rm", "--pull=never"}
	if s.Name != "" {
		argv = append(argv, "--name="+s.Name)
	}
	if s.ReadOnlyRoot {
		argv = append(argv, "--read-only")
	}
	for _, c := range s.CapDrop {
		argv = append(argv, "--cap-drop="+c)
	}
	for _, o := range s.SecurityOpt {
		argv = append(argv, "--security-opt="+o)
	}
	argv = append(argv,
		"--user="+s.User,
		"--pids-limit="+strconv.Itoa(s.PidsLimit),
		"--memory="+strconv.Itoa(s.MemoryMB)+"m",
		"--memory-swap="+strconv.Itoa(s.MemoryMB)+"m",
		"--cpus="+s.CPUs,
		"--ipc=private",
		"--hostname=lr-cloud-step",
	)
	argv = append(argv, s.NetworkFlags...)
	for _, m := range s.Mounts {
		flag := "--mount=type=bind,source=" + m.Source + ",target=" + m.Target
		if !m.ReadWrite {
			flag += ",readonly"
		}
		argv = append(argv, flag)
	}
	for _, t := range s.Tmpfs {
		argv = append(argv, "--tmpfs="+t.Target+":"+t.Options)
	}
	for _, e := range s.Env {
		argv = append(argv, "--env="+e)
	}
	argv = append(argv, "--workdir="+s.Workdir, "--entrypoint="+s.Entrypoint, s.Image)
	return append(argv, s.Args...)
}

const redacted = "[REDACTED]"

// RedactLog masks every secret in a step log, including each line of a multi-line key.
// Short fragments are skipped so ordinary words are not blanked out.
func RedactLog(log string, secrets ...string) string {
	var needles []string
	for _, s := range secrets {
		if s == "" {
			continue
		}
		needles = append(needles, s)
		for _, line := range strings.Split(s, "\n") {
			if line = strings.TrimSpace(line); len(line) >= 8 {
				needles = append(needles, line)
			}
		}
	}
	// Longest first so a whole value is masked before its pieces.
	sort.SliceStable(needles, func(i, j int) bool { return len(needles[i]) > len(needles[j]) })
	for _, n := range needles {
		log = strings.ReplaceAll(log, n, redacted)
	}
	return log
}
