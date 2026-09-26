package importplan

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrNotDockerRun is returned when the text is not a `docker run` command.
var ErrNotDockerRun = errors.New("importplan: not a docker run command")

// ErrNoImage is returned when a docker run command names no image.
var ErrNoImage = errors.New("importplan: docker run command has no image")

// dockerRun is the parsed form of a docker run command line.
type dockerRun struct {
	Name       string
	Image      string
	Command    []string
	Entrypoint []string
	Env        []EnvVar
	Ports      []PortMapping
	Volumes    []VolumeMount
	Restart    string
	Memory     int64
	NanoCPUs   int64
	Network    string
	Warnings   []Warning
}

// flagsWithValue lists docker run flags that consume the next argument.
var flagsWithValue = map[string]bool{
	"-p": true, "--publish": true, "-e": true, "--env": true, "--env-file": true,
	"-v": true, "--volume": true, "--restart": true, "--name": true,
	"-m": true, "--memory": true, "--cpus": true, "--network": true, "--net": true,
	"--entrypoint": true, "-u": true, "--user": true, "-w": true, "--workdir": true,
	"-h": true, "--hostname": true, "-l": true, "--label": true, "--cap-add": true,
	"--cap-drop": true, "--device": true, "--pid": true, "--ipc": true, "--uts": true,
	"--security-opt": true, "--add-host": true, "--dns": true, "--log-driver": true,
	"--log-opt": true, "--mount": true, "--tmpfs": true, "--ulimit": true, "--gpus": true,
	"--platform": true, "--pull": true, "--cpuset-cpus": true, "--shm-size": true,
	"--health-cmd": true, "--health-interval": true, "--health-retries": true,
	"--health-timeout": true, "--health-start-period": true, "--runtime": true,
	"--volumes-from": true, "--link": true, "--sysctl": true, "--memory-swap": true,
	"--oom-score-adj": true, "--stop-signal": true, "--stop-timeout": true,
	"--group-add": true, "--userns": true, "--cgroupns": true, "--ip": true,
	"--mac-address": true, "--expose": true, "--env-file-cwd": true, "--cidfile": true,
	"--label-file": true, "--dns-search": true, "--dns-option": true, "--network-alias": true,
	"--net-alias": true, "--storage-opt": true, "--device-cgroup-rule": true,
	"--kernel-memory": true, "--memory-reservation": true, "--cpu-shares": true, "-c": true,
	"--blkio-weight": true, "--pids-limit": true, "--restart-max": true, "--annotation": true,
}

// unsupportedFlag explains flags the platform cannot honour.
var unsupportedFlag = map[string]string{
	"--privileged":         "privileged containers are not supported",
	"--cap-add":            "adding Linux capabilities is not supported",
	"--cap-drop":           "dropping Linux capabilities is not supported",
	"--device":             "passing host devices is not supported",
	"--pid":                "sharing a PID namespace is not supported",
	"--ipc":                "sharing an IPC namespace is not supported",
	"--uts":                "sharing a UTS namespace is not supported",
	"--security-opt":       "custom security options are not supported",
	"--userns":             "user namespace overrides are not supported",
	"--cgroupns":           "cgroup namespace overrides are not supported",
	"--sysctl":             "sysctl overrides are not supported",
	"--ulimit":             "ulimit overrides are not supported",
	"--tmpfs":              "tmpfs mounts are not supported",
	"--mount":              "--mount syntax is not supported, use -v",
	"--link":               "legacy container links are not supported",
	"--volumes-from":       "volumes-from is not supported",
	"--ip":                 "static container IPs are not supported",
	"--mac-address":        "custom MAC addresses are not supported",
	"--add-host":           "extra host entries are not supported",
	"--dns":                "custom DNS servers are not supported",
	"--dns-search":         "custom DNS search domains are not supported",
	"--dns-option":         "custom DNS options are not supported",
	"--runtime":            "custom container runtimes are not supported",
	"--gpus":               "GPU requests are not imported, set them on the app after creating it",
	"--shm-size":           "shared memory size is not supported",
	"--group-add":          "supplementary groups are not supported",
	"--log-driver":         "custom log drivers are not supported",
	"--log-opt":            "custom log driver options are not supported",
	"--user":               "running as a specific user is not supported",
	"-u":                   "running as a specific user is not supported",
	"--workdir":            "overriding the working directory is not supported",
	"-w":                   "overriding the working directory is not supported",
	"--hostname":           "custom hostnames are not supported",
	"-h":                   "custom hostnames are not supported",
	"--cpuset-cpus":        "CPU pinning is not imported",
	"--memory-swap":        "swap limits are not imported",
	"--pids-limit":         "PID limits are not supported",
	"--stop-signal":        "custom stop signals are not supported",
	"--stop-timeout":       "custom stop timeouts are not supported",
	"--init":               "--init is not supported",
	"--read-only":          "read-only root filesystems are not supported",
	"--platform":           "platform overrides are not supported",
	"--oom-score-adj":      "OOM score adjustment is not supported",
	"--cpu-shares":         "CPU shares are not supported",
	"--health-cmd":         "custom health commands are not imported, add an HTTP health check after creating the app",
	"--storage-opt":        "storage options are not supported",
	"--network-alias":      "network aliases are not supported",
	"--net-alias":          "network aliases are not supported",
	"--expose":             "--expose is informational, use -p to publish a port",
	"--label":              "labels are not imported",
	"-l":                   "labels are not imported",
	"--label-file":         "labels are not imported",
	"--cidfile":            "container id files are not supported",
	"--annotation":         "annotations are not supported",
	"--kernel-memory":      "kernel memory limits are not supported",
	"--userns-mode":        "user namespace overrides are not supported",
	"--device-cgroup-rule": "device cgroup rules are not supported",
}

// ignoredFlag are accepted silently because they do not change what runs.
var ignoredFlag = map[string]bool{
	"-d": true, "--detach": true, "--rm": true, "-i": true, "--interactive": true,
	"-t": true, "--tty": true, "-it": true, "-dit": true, "-di": true, "-dt": true,
	"--pull": true, "--sig-proxy": true, "--quiet": true, "-q": true, "--no-healthcheck": true,
}

// parseDockerRun parses a `docker run` (or `docker container run`) command.
// Every flag it cannot honour becomes a Warning; none is dropped silently.
func parseDockerRun(text string) (*dockerRun, error) {
	toks, err := Tokenize(text)
	if err != nil {
		return nil, fmt.Errorf("importplan: tokenize docker run: %w", err)
	}
	i := 0
	for i < len(toks) && (toks[i] == "sudo" || strings.Contains(toks[i], "=") && !strings.HasPrefix(toks[i], "-")) {
		i++
	}
	if i >= len(toks) || toks[i] != "docker" {
		return nil, ErrNotDockerRun
	}
	i++
	if i < len(toks) && toks[i] == "container" {
		i++
	}
	if i >= len(toks) || toks[i] != "run" {
		return nil, ErrNotDockerRun
	}
	i++

	dr := &dockerRun{}
	warn := func(code, msg string) { dr.Warnings = append(dr.Warnings, Warning{Code: code, Message: msg}) }
	unsupported := func(flag string) {
		reason, ok := unsupportedFlag[flag]
		if !ok {
			reason = "this flag is not supported"
		}
		warn("unsupported_flag", fmt.Sprintf("%s: %s (not supported, ignored)", flag, reason))
	}

	for ; i < len(toks); i++ {
		t := toks[i]
		if t == "--" {
			i++
			break
		}
		if !strings.HasPrefix(t, "-") || t == "-" {
			break
		}
		flag, val, hasEq := strings.Cut(t, "=")
		if !strings.HasPrefix(t, "--") && len(t) > 2 && !hasEq {
			if flagsWithValue[t[:2]] {
				flag, val, hasEq = t[:2], t[2:], true
			} else if allIgnoredShort(t) {
				continue
			}
		}
		if flagsWithValue[flag] && !hasEq {
			if i+1 >= len(toks) {
				return nil, fmt.Errorf("importplan: flag %s needs a value", flag)
			}
			i++
			val = toks[i]
		}
		if err := dr.applyFlag(flag, val, unsupported, warn); err != nil {
			return nil, err
		}
	}
	if i >= len(toks) {
		return nil, ErrNoImage
	}
	dr.Image = toks[i]
	if i+1 < len(toks) {
		dr.Command = toks[i+1:]
	}
	return dr, nil
}

func allIgnoredShort(t string) bool {
	for _, c := range t[1:] {
		if c != 'd' && c != 'i' && c != 't' && c != 'q' {
			return false
		}
	}
	return true
}

func (dr *dockerRun) applyFlag(flag, val string, unsupported func(string), warn func(code, msg string)) error {
	switch flag {
	case "-p", "--publish":
		p, err := parsePublish(val)
		if err != nil {
			return err
		}
		dr.Ports = append(dr.Ports, p)
	case "-e", "--env":
		k, v, hasEq := strings.Cut(val, "=")
		if !envKeyRe.MatchString(k) {
			return fmt.Errorf("importplan: invalid env var name %q", k)
		}
		if !hasEq {
			dr.Env = append(dr.Env, EnvVar{Key: k, Required: true, Secret: looksSecret(k, ""), Source: "docker run -e"})
			warn("env_from_host", fmt.Sprintf("-e %s takes its value from the host shell, enter a value in the plan", k))
			return nil
		}
		ev := newEnvVar(k, v, "docker run -e")
		if ev.Secret && v != "" {
			ev.Required, ev.HasDefault = true, false
			warn("secret_reenter", fmt.Sprintf("-e %s looks like a secret, so its value is not echoed back; enter it again", k))
		}
		dr.Env = append(dr.Env, ev)
	case "--env-file":
		warn("env_file", fmt.Sprintf("--env-file %s cannot be read from here, paste its variables into the env table", val))
	case "-v", "--volume":
		m, err := parseVolume(val)
		if err != nil {
			return err
		}
		if m.NeedsApproval {
			warn("bind_mount", fmt.Sprintf("bind mount %s needs root approval to create", m.HostPath))
		}
		if m.HostPath == "/var/run/docker.sock" || m.HostPath == "/run/docker.sock" {
			warn("docker_socket", "this mounts the Docker socket, which gives the container control of the host; review before deploying")
		}
		dr.Volumes = append(dr.Volumes, m)
	case "--restart":
		dr.Restart = val
	case "--name":
		dr.Name = val
	case "-m", "--memory":
		b, err := parseBytes(val)
		if err != nil {
			return err
		}
		dr.Memory = b
	case "--cpus":
		f, err := strconv.ParseFloat(val, 64)
		if err != nil || f <= 0 {
			return fmt.Errorf("importplan: invalid --cpus %q", val)
		}
		dr.NanoCPUs = int64(f * 1e9)
	case "--network", "--net":
		dr.Network = val
		if val == "host" {
			warn("unsupported_flag", "--network host: host networking is not supported (not supported, ignored); ports are published instead")
		} else if val != "bridge" && val != "" {
			warn("network", fmt.Sprintf("--network %s: custom networks are not imported, the app joins the platform network", val))
		}
	case "--entrypoint":
		ep, err := Tokenize(val)
		if err != nil {
			return fmt.Errorf("importplan: --entrypoint: %w", err)
		}
		dr.Entrypoint = ep
	case "--pid":
		if val == "host" {
			warn("unsupported_flag", "--pid host: sharing the host PID namespace is not supported (not supported, ignored)")
		} else {
			unsupported(flag)
		}
	case "--privileged":
		unsupported(flag)
	default:
		if ignoredFlag[flag] {
			return nil
		}
		unsupported(flag)
	}
	return nil
}

// parsePublish parses [ip:][host:]container[/proto].
func parsePublish(s string) (PortMapping, error) {
	s, _, _ = strings.Cut(s, "/")
	parts := strings.Split(s, ":")
	var host, cont string
	switch len(parts) {
	case 1:
		cont = parts[0]
	case 2:
		host, cont = parts[0], parts[1]
	case 3:
		host, cont = parts[1], parts[2]
	default:
		return PortMapping{}, fmt.Errorf("importplan: invalid port mapping %q", s)
	}
	c, err := strconv.Atoi(cont)
	if err != nil || c < 1 || c > 65535 {
		return PortMapping{}, fmt.Errorf("importplan: invalid container port in %q", s)
	}
	pm := PortMapping{Container: c}
	if host != "" {
		h, err := strconv.Atoi(host)
		if err != nil || h < 1 || h > 65535 {
			return PortMapping{}, fmt.Errorf("importplan: invalid host port in %q", s)
		}
		pm.Host = h
	}
	return pm, nil
}

// parseVolume parses src:dst[:opts]; an absolute or relative path source is a bind mount.
func parseVolume(s string) (VolumeMount, error) {
	parts := strings.Split(s, ":")
	if len(parts) == 1 {
		if !strings.HasPrefix(parts[0], "/") {
			return VolumeMount{}, fmt.Errorf("importplan: invalid volume %q", s)
		}
		return VolumeMount{ContainerPath: parts[0]}, nil
	}
	m := VolumeMount{ContainerPath: parts[1]}
	if !strings.HasPrefix(m.ContainerPath, "/") {
		return VolumeMount{}, fmt.Errorf("importplan: volume %q: container path must be absolute", s)
	}
	if len(parts) > 2 {
		for _, o := range strings.Split(parts[2], ",") {
			if o == "ro" {
				m.ReadOnly = true
			}
		}
	}
	src := parts[0]
	if strings.HasPrefix(src, "/") || strings.HasPrefix(src, ".") || strings.HasPrefix(src, "~") {
		m.HostPath = src
		m.NeedsApproval = true
	} else {
		m.Name = src
	}
	return m, nil
}

// parseBytes parses 512m, 2g, 1.5g, 1024 (bytes) style sizes.
func parseBytes(s string) (int64, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	mult := int64(1)
	for _, u := range []struct {
		suf string
		m   int64
	}{{"gb", 1 << 30}, {"g", 1 << 30}, {"mb", 1 << 20}, {"m", 1 << 20}, {"kb", 1 << 10}, {"k", 1 << 10}, {"b", 1}} {
		if strings.HasSuffix(s, u.suf) {
			mult = u.m
			s = strings.TrimSuffix(s, u.suf)
			break
		}
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f <= 0 {
		return 0, fmt.Errorf("importplan: invalid memory size %q", s)
	}
	return int64(f * float64(mult)), nil
}
