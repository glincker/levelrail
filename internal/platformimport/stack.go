package platformimport

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/spec"
	"gopkg.in/yaml.v3"
)

// Compose app verdicts.
const (
	ComposeReady   = "ready"
	ComposeBlocked = "blocked"
)

// ImportComposeMaxEnv overrides the largest compose file an import accepts, in bytes.
const ImportComposeMaxEnv = "APP_IMPORT_COMPOSE_MAX_BYTES"

const (
	defaultImportComposeMax = 256 * 1024
	importComposeHardMax    = 4 << 20
	importFileHardMax       = 1 << 20
)

// ImportComposeMax returns the compose size cap, from ImportComposeMaxEnv when it is a positive integer.
func ImportComposeMax() int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(ImportComposeMaxEnv))); err == nil && v > 0 {
		return min(v, importComposeHardMax)
	}
	return defaultImportComposeMax
}

// ComposeApp is a multi-service stack imported as one compose app. Compose is the source text
// and may hold inline values, so it is never serialized; Env carries the secret flag per key.
type ComposeApp struct {
	SourceID    string   `json:"source_id"`
	Name        string   `json:"name"`
	Project     string   `json:"project,omitempty"`
	Environment string   `json:"environment,omitempty"`
	Compose     string   `json:"-"`
	Env         []Env    `json:"-"`
	Domains     []string `json:"domains,omitempty"`
	Volumes     []string `json:"volumes,omitempty"`
	Verdict     string   `json:"verdict"`
	Blocked     []string `json:"blocked,omitempty"`
	Notes       []Note   `json:"notes,omitempty"`
}

// SplitEnv returns the plain values and the secret values of the stack env, never mixed.
func (c ComposeApp) SplitEnv() (plain, secrets map[string]string) {
	plain, secrets = map[string]string{}, map[string]string{}
	for _, e := range c.Env {
		if e.Key == "" {
			continue
		}
		if e.Secret {
			secrets[e.Key] = e.Value
			continue
		}
		plain[e.Key] = e.Value
	}
	return plain, secrets
}

// NewComposeApp checks body with the same validation a manual compose deploy runs and
// sets the verdict, so an imported stack is never more permissive than a pasted one.
func NewComposeApp(sourceID, name, project, body string, env []Env, domains []string) ComposeApp {
	out := ComposeApp{SourceID: sourceID, Name: name, Project: project, Compose: body, Env: env, Verdict: ComposeReady}
	for _, h := range domains {
		if host, ok := cleanImportHost(h); ok {
			out.Domains = append(out.Domains, host)
			continue
		}
		out.Notes = append(out.Notes, Note{Area: "domain", Reason: "a domain entry is not a plain host name and was dropped", Manual: "add the domain by hand after import"})
	}
	out.Volumes, out.Blocked = CheckCompose(body)
	if len(out.Blocked) > 0 {
		out.Verdict = ComposeBlocked
	}
	return out
}

// CheckCompose parses and validates an untrusted compose body. It returns the named
// volumes it declares and one reason per rule it breaks; no file or network is read.
func CheckCompose(body string) (volumes, blocked []string) {
	if strings.TrimSpace(body) == "" {
		return nil, []string{"the stack has no readable compose file"}
	}
	if limit := ImportComposeMax(); len(body) > limit {
		return nil, []string{fmt.Sprintf("the compose file is %d bytes, over the %d byte import limit (%s)", len(body), limit, ImportComposeMaxEnv)}
	}
	f, err := compose.Parse([]byte(body))
	if err != nil {
		return nil, append([]string{err.Error()}, HostAccessBlocked(body)...)
	}
	if err := f.Validate(); err != nil {
		blocked = splitErrors(err)
	}
	seen := map[string]bool{}
	for _, svc := range f.Services {
		for _, v := range svc.Volumes {
			if v.HostPath == "" && v.Name != "" && !seen[v.Name] {
				seen[v.Name] = true
				volumes = append(volumes, v.Name)
			}
		}
	}
	sort.Strings(volumes)
	return volumes, blocked
}

// splitErrors turns the compose validator's joined message back into one reason per rule.
func splitErrors(err error) []string {
	lines := strings.Split(err.Error(), "\n  - ")
	if len(lines) > 1 {
		return lines[1:]
	}
	return lines
}

var importHostRe = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?)*$`)

// cleanImportHost keeps a bare host; a scheme is stripped, a URL with userinfo is refused.
func cleanImportHost(raw string) (string, bool) {
	h := strings.TrimSpace(raw)
	if i := strings.Index(h, "://"); i >= 0 {
		h = h[i+3:]
	}
	if strings.Contains(h, "@") {
		return "", false
	}
	if i := strings.IndexAny(h, "/?#"); i >= 0 {
		h = h[:i]
	}
	if i := strings.LastIndex(h, ":"); i >= 0 {
		h = h[:i]
	}
	h = strings.ToLower(h)
	return h, h != "" && importHostRe.MatchString(h)
}

// AppFile is a config file mounted into an app. Content is never serialized; a secret
// file goes through the encrypted secret store when it is created.
type AppFile struct {
	Path    string `json:"path"`
	Content []byte `json:"-"`
	Secret  bool   `json:"secret,omitempty"`
}

var secretFileNameRe = regexp.MustCompile(`(?i)(secret|passw|token|credential|private|\.key$|\.pem$|\.p12$|\.pfx$|id_rsa|id_ed25519|\.env$|htpasswd|\.kdbx$)`)

var secretContentRe = regexp.MustCompile(`(?i)(pass|secret|token|api_?key|credential|private[_ ]?key|auth)\w*["']?\s*[:=]|-----BEGIN [A-Z0-9 ]*(PRIVATE KEY|CERTIFICATE)|[a-z][a-z0-9+.-]*://[^\s/@]*:[^\s/@]+@`)

// FileLooksSecret reports whether a file name or its content suggests a credential.
func FileLooksSecret(p string, content []byte) bool {
	if secretFileNameRe.MatchString(p) {
		return true
	}
	return secretContentRe.Match(content)
}

// NewAppFile validates an untrusted file mount: the path must be a clean absolute file path
// and the content within the configured size cap. Bytes are kept exactly as read.
func NewAppFile(mountPath string, content []byte) (AppFile, error) {
	if strings.ContainsAny(mountPath, "\\\x00\r") {
		return AppFile{}, fmt.Errorf("path %q contains a backslash or control character", mountPath)
	}
	if err := spec.ValidateFilePath(mountPath); err != nil {
		return AppFile{}, err
	}
	for _, seg := range strings.Split(mountPath, "/") {
		if seg == "." {
			return AppFile{}, fmt.Errorf("path %q must not have a dot segment", mountPath)
		}
	}
	if !utf8.Valid(content) {
		return AppFile{}, fmt.Errorf("file %q is binary, app files hold text only", mountPath)
	}
	if limit := min(spec.FileLimitsFromEnv().MaxBytes, importFileHardMax); len(content) > limit {
		return AppFile{}, fmt.Errorf("file %q is %d bytes, over the %d byte limit", mountPath, len(content), limit)
	}
	return AppFile{Path: mountPath, Content: append([]byte(nil), content...), Secret: FileLooksSecret(mountPath, content)}, nil
}

// hostCheckFile is a lenient typed decode that reads only the host-access keys and bind sources.
type hostCheckFile struct {
	Services map[string]struct {
		compose.HostAccess `yaml:",inline"`
		Volumes            []yaml.Node `yaml:"volumes"`
	} `yaml:"services"`
}

// HostAccessBlocked enforces the compose host-access rules even on a file the full parser
// rejects, so a privileged service or socket mount is never staged as ready.
func HostAccessBlocked(body string) []string {
	var f hostCheckFile
	if err := yaml.Unmarshal([]byte(body), &f); err != nil {
		return []string{"the compose file is not valid YAML: " + err.Error()}
	}
	names := make([]string, 0, len(f.Services))
	for n := range f.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	var out []string
	for _, n := range names {
		svc := f.Services[n]
		var hostPaths []string
		for _, v := range svc.Volumes {
			switch v.Kind {
			case yaml.ScalarNode:
				if src, _, ok := strings.Cut(v.Value, ":"); ok {
					hostPaths = append(hostPaths, src)
				}
			case yaml.MappingNode:
				var long struct {
					Source string `yaml:"source"`
				}
				if v.Decode(&long) == nil && long.Source != "" {
					hostPaths = append(hostPaths, long.Source)
				}
			}
		}
		for _, msg := range compose.ServiceHostViolations(svc.HostAccess, hostPaths) {
			out = append(out, fmt.Sprintf("service %q: %s", n, msg))
		}
	}
	return out
}
