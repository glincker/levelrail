package compose

import (
	"fmt"
	"net/netip"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/dockerguard"
	"gopkg.in/yaml.v3"
)

// Stable refusal codes for referenced compose files. The IaC layer maps them
// onto its own code list, so the strings must not change.
const (
	CodeStrictTooLarge       = "compose_too_large"
	CodeStrictUnsupportedKey = "compose_unsupported_key"
	CodeStrictRejected       = "compose_rejected"
	CodeStrictSecretLiteral  = "compose_secret_literal"
	CodeStrictVarMissing     = "compose_var_missing"
)

// Env seeds for the strict caps. Bytes are checked before any parse; nodes
// count aliases as expanded, so a billion laughs file fails without expanding.
const (
	envStrictMaxBytes     = "APP_COMPOSE_STRICT_MAX_BYTES"
	envStrictMaxNodes     = "APP_COMPOSE_STRICT_MAX_NODES"
	defaultStrictMaxBytes = 256 << 10
	defaultStrictMaxNodes = 250000
)

// StrictLimits bounds one referenced compose file.
type StrictLimits struct {
	MaxBytes int
	MaxNodes int
}

// LoadStrictLimits reads the caps from env, falling back to the defaults on
// unset or invalid values.
func LoadStrictLimits() StrictLimits {
	return StrictLimits{
		MaxBytes: envPositiveInt(envStrictMaxBytes, defaultStrictMaxBytes),
		MaxNodes: envPositiveInt(envStrictMaxNodes, defaultStrictMaxNodes),
	}
}

func envPositiveInt(name string, fallback int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name))); err == nil && v > 0 {
		return v
	}
	return fallback
}

// StrictError is one refusal. Message carries names and lines only, never a value.
type StrictError struct {
	Code    string
	Rule    string
	Service string
	Key     string
	Line    int
	Message string
}

func (e *StrictError) Error() string {
	var b strings.Builder
	if e.Line > 0 {
		fmt.Fprintf(&b, "line %d: ", e.Line)
	}
	if e.Service != "" {
		fmt.Fprintf(&b, "service %q: ", e.Service)
	}
	b.WriteString(e.Message)
	if e.Rule != "" {
		fmt.Fprintf(&b, " [%s]", e.Rule)
	}
	return b.String()
}

// StrictErrors is every refusal for one file, so an author fixes them in one pass.
type StrictErrors []*StrictError

func (es StrictErrors) Error() string {
	parts := make([]string, len(es))
	for i, e := range es {
		parts[i] = e.Error()
	}
	return "compose: " + strings.Join(parts, "; ")
}

func (es StrictErrors) orNil() error {
	if len(es) == 0 {
		return nil
	}
	return es
}

var strictTopLevel = map[string]bool{"name": true, "services": true, "volumes": true, "networks": true, "version": true}

// ParseStrict parses and validates a compose file that arrived from an
// untrusted source (a referenced IaC attachment). On refusal the error is a
// StrictErrors; the non strict Parse and Validate paths are not affected.
func ParseStrict(data []byte, lim StrictLimits) (*File, error) {
	root, errs := strictRoot(data, lim)
	if len(errs) > 0 {
		return nil, errs
	}
	lines := map[string]int{}
	errs = strictKeys(root, lines)
	if len(errs) > 0 {
		return nil, errs
	}
	f, err := Parse(data)
	if err != nil {
		return nil, StrictErrors{{Code: CodeStrictRejected, Message: err.Error()}}
	}
	// Grants only come from a policy checked manifest, never from the file.
	f.CapGrants, f.DropUngrantedCaps = nil, false
	if len(f.Services) == 0 {
		return nil, StrictErrors{{Code: CodeStrictRejected, Message: "no services declared"}}
	}
	for _, name := range sortedServiceNames(f) {
		svc := f.Services[name]
		for _, v := range svc.Volumes {
			if v.HostPath != "" {
				errs = append(errs, &StrictError{Code: CodeStrictRejected, Rule: dockerguard.RuleBindUndeclared, Service: name, Key: "volumes", Line: lines[name],
					Message: "bind mounts are not allowed in a referenced compose file; use a named volume"})
			}
		}
		for _, h := range svc.ExtraHosts {
			if msg := riskyExtraHost(h); msg != "" {
				errs = append(errs, &StrictError{Code: CodeStrictRejected, Service: name, Key: "extra_hosts", Line: lines[name], Message: msg})
			}
		}
	}
	for _, group := range [][]error{
		validateComposeServices(f, false, true),
		validateComposeDomainRefs(f),
		validateComposeDependsOn(f),
		validateUnsupportedTopLevel(f),
		validateHostAccess(f),
		validateStrategies(f),
	} {
		for _, e := range group {
			errs = append(errs, rejected(e))
		}
	}
	if len(errs) > 0 {
		return nil, errs
	}
	return f, nil
}

var ruleSuffixRe = regexp.MustCompile(`\s*\[([a-z0-9_]+)\]$`)

// rejected keeps the dockerguard rule id the shared guards already cite.
func rejected(err error) *StrictError {
	msg := err.Error()
	e := &StrictError{Code: CodeStrictRejected}
	if m := ruleSuffixRe.FindStringSubmatchIndex(msg); m != nil {
		e.Rule = msg[m[2]:m[3]]
		msg = msg[:m[0]]
	}
	e.Message = msg
	return e
}

// strictRoot enforces the byte cap before parsing, then the expanded node cap
// before anything decodes the document with aliases resolved.
func strictRoot(data []byte, lim StrictLimits) (*yaml.Node, StrictErrors) {
	if lim.MaxBytes <= 0 || lim.MaxNodes <= 0 {
		lim = LoadStrictLimits()
	}
	if len(data) > lim.MaxBytes {
		return nil, StrictErrors{{Code: CodeStrictTooLarge, Message: fmt.Sprintf("file is larger than the %d byte limit", lim.MaxBytes)}}
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, StrictErrors{{Code: CodeStrictRejected, Message: "parse: " + err.Error()}}
	}
	if n := expandedNodes(&doc, map[*yaml.Node]int{}, lim.MaxNodes); n > lim.MaxNodes {
		return nil, StrictErrors{{Code: CodeStrictTooLarge, Message: fmt.Sprintf("document expands past the %d node limit (aliases count as expanded)", lim.MaxNodes)}}
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, StrictErrors{{Code: CodeStrictRejected, Message: "top level must be a mapping"}}
	}
	return doc.Content[0], nil
}

// expandedNodes counts nodes with every alias expanded, memoized per node so
// the work stays linear in the file. A cycle counts as over the limit.
func expandedNodes(n *yaml.Node, memo map[*yaml.Node]int, limit int) int {
	if n == nil {
		return 0
	}
	if v, ok := memo[n]; ok {
		if v < 0 {
			return limit + 1
		}
		return v
	}
	memo[n] = -1
	total := 1
	if n.Kind == yaml.AliasNode {
		total += expandedNodes(n.Alias, memo, limit)
	}
	for _, c := range n.Content {
		if total > limit {
			break
		}
		total += expandedNodes(c, memo, limit)
	}
	if total > limit {
		total = limit + 1
	}
	memo[n] = total
	return total
}

func resolveAlias(n *yaml.Node) *yaml.Node {
	for i := 0; n != nil && n.Kind == yaml.AliasNode && i < 64; i++ {
		n = n.Alias
	}
	return n
}

// strictKeys refuses the keys that name other files or a build context, and
// records each service's line for later messages.
func strictKeys(root *yaml.Node, lines map[string]int) StrictErrors {
	var errs StrictErrors
	for i := 0; i+1 < len(root.Content); i += 2 {
		k, v := root.Content[i], resolveAlias(root.Content[i+1])
		switch {
		case k.Value == "include":
			errs = append(errs, &StrictError{Code: CodeStrictUnsupportedKey, Key: k.Value, Line: k.Line, Message: "include: is not supported; one compose file per app"})
		case k.Value == "services":
			errs = append(errs, strictServices(v, lines)...)
		case strictTopLevel[k.Value] || strings.HasPrefix(k.Value, "x-"):
		default:
			errs = append(errs, &StrictError{Code: CodeStrictUnsupportedKey, Key: k.Value, Line: k.Line, Message: fmt.Sprintf("top level key %q is not supported in a referenced compose file", k.Value)})
		}
	}
	return errs
}

func strictServices(services *yaml.Node, lines map[string]int) StrictErrors {
	if services == nil || services.Kind != yaml.MappingNode {
		return nil
	}
	var errs StrictErrors
	for i := 0; i+1 < len(services.Content); i += 2 {
		name, body := services.Content[i].Value, resolveAlias(services.Content[i+1])
		lines[name] = services.Content[i].Line
		if body == nil || body.Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j+1 < len(body.Content); j += 2 {
			k, v := body.Content[j], resolveAlias(body.Content[j+1])
			refuse := func(msg string) {
				errs = append(errs, &StrictError{Code: CodeStrictUnsupportedKey, Service: name, Key: k.Value, Line: k.Line, Message: msg})
			}
			switch k.Value {
			case "build":
				refuse("build: is not supported; reference a pushed image")
			case "env_file":
				refuse("env_file: is not supported; declare values in spec.compose.env")
			case "extends":
				if v != nil && v.Kind == yaml.MappingNode && mappingHas(v, "file") {
					refuse("extends.file is not supported; only same file extends works")
				}
			}
		}
	}
	return errs
}

func mappingHas(m *yaml.Node, key string) bool {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return true
		}
	}
	return false
}

// riskyExtraHost refuses targets that reach the host or the metadata endpoint.
func riskyExtraHost(entry string) string {
	sep := strings.IndexAny(entry, ":=")
	if sep < 0 {
		return "extra_hosts entries must be host:ip"
	}
	target := strings.Trim(strings.TrimSpace(entry[sep+1:]), "[]")
	if strings.EqualFold(target, "host-gateway") {
		return "extra_hosts: host-gateway reaches the host and is not allowed"
	}
	ip, err := netip.ParseAddr(target)
	if err != nil {
		return "extra_hosts targets must be IP addresses"
	}
	ip = ip.Unmap()
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsLinkLocalMulticast() {
		return "extra_hosts: loopback, link local and unspecified targets are not allowed (host and metadata reach)"
	}
	return ""
}
