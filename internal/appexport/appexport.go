// Package appexport renders an app as plain Docker Compose plus an env
// template so leaving Levelrail never needs Levelrail. Secret values are
// never part of the input type; the caller classifies env before export.
package appexport

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// SecretPlaceholder stands in for every value that must not leave the server.
const SecretPlaceholder = "CHANGE_ME"

// Volume is a named volume mounted into a service.
type Volume struct {
	Name          string
	ContainerPath string
}

// BindMount is a host path mount; it is exported as is and flagged in the README.
type BindMount struct {
	HostPath      string
	ContainerPath string
	ReadOnly      bool
}

// Port is an extra host-published port.
type Port struct {
	HostPort      int
	ContainerPort int
	Protocol      string
}

// Probe is the readiness probe of a service.
type Probe struct {
	Path     string
	Scheme   string
	TCP      bool
	Exec     []string
	Interval string
	Timeout  string
	Failures int
}

// Service is one container of the app. Env holds only non-secret values as
// stored; SecretEnv and Refs hold names and descriptions, never values.
type Service struct {
	Key              string
	Image            string
	Command          []string
	Entrypoint       []string
	User             string
	WorkingDir       string
	Port             int
	Domains          []string
	Env              map[string]string
	SecretEnv        []string
	Refs             map[string]string
	Volumes          []Volume
	BindMounts       []BindMount
	Ports            []Port
	Health           *Probe
	DependsOn        []string
	DependsOnHealthy []string
	Restart          string
	MemoryBytes      int64
	NanoCPUs         int64
	PullPolicy       string
	FilePaths        []string
	// Unsupported lists settings the caller could not express in compose; a
	// project export prints them in its header instead of dropping them.
	Unsupported []string
	// SecretValues holds opt-in plaintext for secret keys, used only by a
	// project export that was explicitly asked to include secrets.
	SecretValues map[string]string
}

// Input is everything one export needs.
type Input struct {
	AppName  string
	Services []Service
}

// File is one produced file.
type File struct {
	Name    string
	Content []byte
}

var (
	composeName  = regexp.MustCompile(`[^a-z0-9_-]+`)
	safeKey      = regexp.MustCompile(`[^A-Za-z0-9_]`)
	envNameRe    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	serviceKeyRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
)

// ErrInvalidName marks a service key or env name that cannot be written to
// compose or .env without breaking the file structure.
var ErrInvalidName = errors.New("appexport: invalid service or variable name")

func validateNames(svcs []Service) error {
	for _, s := range svcs {
		if !serviceKeyRe.MatchString(s.Key) {
			return fmt.Errorf("%w: service key %q", ErrInvalidName, s.Key)
		}
		for _, d := range s.DependsOn {
			if !serviceKeyRe.MatchString(d) {
				return fmt.Errorf("%w: depends_on %q", ErrInvalidName, d)
			}
		}
		for _, k := range allKeys(s) {
			if !envNameRe.MatchString(k) {
				return fmt.Errorf("%w: env name %q in service %q", ErrInvalidName, k, s.Key)
			}
		}
	}
	return nil
}

// Render produces docker-compose.yml, .env.example and README.md, sorted and
// byte-stable for the same input.
func Render(in Input) ([]File, error) {
	if in.AppName == "" || len(in.Services) == 0 {
		return nil, fmt.Errorf("appexport: an app name and at least one service are required")
	}
	svcs := append([]Service(nil), in.Services...)
	sort.Slice(svcs, func(i, j int) bool { return svcs[i].Key < svcs[j].Key })
	seen := map[string]bool{}
	for _, s := range svcs {
		if s.Key == "" || s.Image == "" || seen[s.Key] {
			return nil, fmt.Errorf("appexport: service %q needs a unique key and an image", s.Key)
		}
		seen[s.Key] = true
	}
	if err := validateNames(svcs); err != nil {
		return nil, err
	}
	vars := planVars(svcs)
	env := renderEnv(svcs, vars)
	return []File{
		{Name: ".env.example", Content: []byte(env)},
		{Name: "README.md", Content: []byte(renderReadme(in.AppName, svcs))},
		{Name: "docker-compose.yml", Content: []byte(renderCompose(in.AppName, svcs, vars))},
	}, nil
}

// WriteTar writes files as a deterministic tar stream (fixed mtime and owner).
func WriteTar(w io.Writer, files []File) error {
	tw := tar.NewWriter(w)
	for _, f := range files {
		mode := int64(0o644)
		if f.Name == ".env" {
			mode = 0o600
		}
		h := &tar.Header{Name: f.Name, Mode: mode, Size: int64(len(f.Content)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if _, err := tw.Write(f.Content); err != nil {
			return err
		}
	}
	return tw.Close()
}

// envVar is one resolved env entry of a service.
type envVar struct {
	Name   string // variable name in .env
	Key    string // key inside the container
	Value  string
	Secret bool
	Note   string
}

// planVars resolves, per service, every env key to a .env variable name. A
// key used by several services is prefixed with the service key so values
// cannot collide.
func planVars(svcs []Service) map[string][]envVar {
	count := map[string]int{}
	for _, s := range svcs {
		for _, k := range allKeys(s) {
			count[k]++
		}
	}
	out := map[string][]envVar{}
	for _, s := range svcs {
		secrets := map[string]bool{}
		for _, k := range s.SecretEnv {
			secrets[k] = true
		}
		for _, k := range allKeys(s) {
			v := envVar{Key: k, Name: k}
			if count[k] > 1 {
				v.Name = strings.ToUpper(safeKey.ReplaceAllString(s.Key, "_")) + "_" + k
			}
			if ref, ok := s.Refs[k]; ok {
				v.Secret, v.Note = true, ref
			} else if secrets[k] {
				v.Secret = true
			} else if val, ok := s.Env[k]; ok {
				v.Value = val
			}
			out[s.Key] = append(out[s.Key], v)
		}
	}
	return out
}

func allKeys(s Service) []string {
	set := map[string]bool{}
	for k := range s.Env {
		set[k] = true
	}
	for _, k := range s.SecretEnv {
		set[k] = true
	}
	for k := range s.Refs {
		set[k] = true
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func renderEnv(svcs []Service, vars map[string][]envVar) string {
	var b strings.Builder
	b.WriteString("# Copy to .env and fill in the secrets before `docker compose up`.\n")
	for _, s := range svcs {
		if len(vars[s.Key]) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n# service: %s\n", s.Key)
		for _, v := range vars[s.Key] {
			switch {
			case v.Secret && v.Note != "":
				fmt.Fprintf(&b, "# secret (%s): re-enter or restore from your backup\n%s=%s\n", v.Note, v.Name, SecretPlaceholder)
			case v.Secret:
				fmt.Fprintf(&b, "# secret: re-enter or restore from your backup\n%s=%s\n", v.Name, SecretPlaceholder)
			default:
				fmt.Fprintf(&b, "%s=%s\n", v.Name, dotenvQuote(v.Value))
			}
		}
	}
	return b.String()
}

// dotenvQuote quotes only when the value needs it; compose's own .env rules apply.
func dotenvQuote(v string) string {
	if v == "" || !strings.ContainsAny(v, " \t\n\r#\"'$\\`") {
		return v
	}
	if strings.Contains(v, `\`) {
		// Single quotes keep backslashes literal; a value with both kinds
		// cannot be quoted safely, so it becomes a placeholder.
		if strings.Contains(v, "'") {
			return SecretPlaceholder
		}
		return "'" + v + "'"
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "$", `\$`, "\n", `\n`, "\r", `\r`)
	return `"` + r.Replace(v) + `"`
}

// ys renders a YAML double-quoted scalar; the compose `$` is escaped so
// literal text never turns into interpolation.
func ys(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(strings.ReplaceAll(s, "$", "$$"))
	return strings.TrimSpace(buf.String())
}

func ysList(items []string) string {
	q := make([]string, len(items))
	for i, it := range items {
		q[i] = ys(it)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

func renderCompose(app string, svcs []Service, vars map[string][]envVar) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Exported from Levelrail app %q. Plain Docker Compose, no Levelrail needed.\n", app)
	b.WriteString("# Domains are kept as x-domains so you can map them in your own proxy.\n")
	fmt.Fprintf(&b, "name: %s\n", ys(projectName(app)))
	b.WriteString("services:\n")
	volumes := map[string]bool{}
	for _, s := range svcs {
		writeService(&b, s, vars[s.Key])
		for _, v := range s.Volumes {
			volumes[volumeKey(v.Name)] = true
		}
	}
	if len(volumes) > 0 {
		b.WriteString("\nvolumes:\n")
		names := make([]string, 0, len(volumes))
		for n := range volumes {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			fmt.Fprintf(&b, "  %s: {}\n", n)
		}
	}
	return b.String()
}

// projectName is the Compose project name, which prefixes every named volume.
func projectName(app string) string {
	n := strings.Trim(composeName.ReplaceAllString(strings.ToLower(app), "-"), "-_")
	if n == "" {
		return "app"
	}
	return n
}

func volumeKey(name string) string {
	k := safeKey.ReplaceAllString(name, "_")
	if k == "" {
		return "data"
	}
	return k
}

func writeService(b *strings.Builder, s Service, vars []envVar) {
	fmt.Fprintf(b, "  %s:\n", s.Key)
	fmt.Fprintf(b, "    image: %s\n", ys(s.Image))
	if s.PullPolicy == "always" {
		b.WriteString("    pull_policy: always\n")
	}
	if len(s.Entrypoint) > 0 {
		fmt.Fprintf(b, "    entrypoint: %s\n", ysList(s.Entrypoint))
	}
	if len(s.Command) > 0 {
		fmt.Fprintf(b, "    command: %s\n", ysList(s.Command))
	}
	if s.User != "" {
		fmt.Fprintf(b, "    user: %s\n", ys(s.User))
	}
	if s.WorkingDir != "" {
		fmt.Fprintf(b, "    working_dir: %s\n", ys(s.WorkingDir))
	}
	fmt.Fprintf(b, "    restart: %s\n", restartPolicy(s.Restart))
	if len(vars) > 0 {
		b.WriteString("    environment:\n")
		for _, v := range vars {
			fmt.Fprintf(b, "      %s: ${%s}\n", v.Key, v.Name)
		}
	}
	var ports []string
	if s.Port > 0 {
		ports = append(ports, fmt.Sprintf("127.0.0.1:%d:%d", s.Port, s.Port))
	}
	for _, p := range s.Ports {
		if p.HostPort == s.Port && p.ContainerPort == s.Port && (p.Protocol == "" || p.Protocol == "tcp") {
			continue
		}
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		ports = append(ports, fmt.Sprintf("%d:%d/%s", p.HostPort, p.ContainerPort, proto))
	}
	if len(ports) > 0 {
		if s.Port > 0 {
			b.WriteString("    # Bound to loopback so the app answers on this host without editing; put a proxy in front or change the bind to expose it.\n")
		}
		b.WriteString("    ports:\n")
		for _, p := range ports {
			fmt.Fprintf(b, "      - %s\n", ys(p))
		}
	} else if s.Port > 0 {
		fmt.Fprintf(b, "    expose: [%s]\n", ys(strconv.Itoa(s.Port)))
	}
	if len(s.Volumes)+len(s.BindMounts) > 0 {
		b.WriteString("    volumes:\n")
		vs := append([]Volume(nil), s.Volumes...)
		sort.Slice(vs, func(i, j int) bool { return vs[i].ContainerPath < vs[j].ContainerPath })
		for _, v := range vs {
			fmt.Fprintf(b, "      - %s\n", ys(volumeKey(v.Name)+":"+v.ContainerPath))
		}
		bm := append([]BindMount(nil), s.BindMounts...)
		sort.Slice(bm, func(i, j int) bool { return bm[i].ContainerPath < bm[j].ContainerPath })
		for _, m := range bm {
			spec := m.HostPath + ":" + m.ContainerPath
			if m.ReadOnly {
				spec += ":ro"
			}
			fmt.Fprintf(b, "      - %s\n", ys(spec))
		}
	}
	if s.Health != nil {
		writeHealth(b, s)
	}
	writeDepends(b, s)
	if s.MemoryBytes > 0 {
		fmt.Fprintf(b, "    mem_limit: %d\n", s.MemoryBytes)
	}
	if s.NanoCPUs > 0 {
		fmt.Fprintf(b, "    cpus: %s\n", strconv.FormatFloat(float64(s.NanoCPUs)/1e9, 'f', -1, 64))
	}
	if len(s.Domains) > 0 {
		d := append([]string(nil), s.Domains...)
		sort.Strings(d)
		fmt.Fprintf(b, "    x-domains: %s\n", ysList(d))
	}
}

func restartPolicy(r string) string {
	switch r {
	case "never", "no":
		return `"no"`
	case "always":
		return r
	default:
		if strings.HasPrefix(r, "on-failure") {
			return r
		}
		return "unless-stopped"
	}
}

func writeHealth(b *strings.Builder, s Service) {
	h := s.Health
	var test string
	switch {
	case len(h.Exec) > 0:
		test = "[\"CMD\", " + strings.TrimSuffix(strings.TrimPrefix(ysList(h.Exec), "["), "]") + "]"
	case h.TCP && s.Port > 0:
		test = ysList([]string{"CMD-SHELL", fmt.Sprintf("nc -z localhost %d || exit 1", s.Port)})
	case h.Path != "" && s.Port > 0:
		scheme := h.Scheme
		if scheme == "" {
			scheme = "http"
		}
		url := fmt.Sprintf("%s://localhost:%d%s", scheme, s.Port, h.Path)
		test = ysList([]string{"CMD", "curl", "-fsS", url})
	default:
		return
	}
	b.WriteString("    healthcheck:\n")
	fmt.Fprintf(b, "      test: %s\n", test)
	if h.Interval != "" {
		fmt.Fprintf(b, "      interval: %s\n", h.Interval)
	}
	if h.Timeout != "" {
		fmt.Fprintf(b, "      timeout: %s\n", h.Timeout)
	}
	if h.Failures > 0 {
		fmt.Fprintf(b, "      retries: %d\n", h.Failures)
	}
}

func writeDepends(b *strings.Builder, s Service) {
	if len(s.DependsOn) == 0 {
		return
	}
	healthy := map[string]bool{}
	for _, d := range s.DependsOnHealthy {
		healthy[d] = true
	}
	deps := append([]string(nil), s.DependsOn...)
	sort.Strings(deps)
	b.WriteString("    depends_on:\n")
	for _, d := range deps {
		cond := "service_started"
		if healthy[d] {
			cond = "service_healthy"
		}
		fmt.Fprintf(b, "      %s:\n        condition: %s\n", d, cond)
	}
}
