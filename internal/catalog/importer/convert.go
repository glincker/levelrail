package importer

import (
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	idPattern         = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	domainDeclPattern = regexp.MustCompile(`^SERVICE_(URL|FQDN)_(.+)$`)
	headerPattern     = regexp.MustCompile(`^#\s*([a-z_]+):\s*(.*)$`)
	imageTagPattern   = regexp.MustCompile(`:[^/:@]+$|@sha256:`)
)

// hostAccessKeys are passed through untouched so the platform's own

// Docker guard rules, not this converter, decide whether they are allowed.

var hostAccessKeys = map[string]bool{
	"privileged": true, "network_mode": true, "pid": true, "ipc": true, "cap_add": true,
	"devices": true, "security_opt": true, "volumes_from": true,
}

// unsupportedKeys change runtime behavior and have no equivalent here,

// so a template that sets one is skipped instead of silently altered.

var unsupportedKeys = map[string]bool{
	"user": true, "working_dir": true, "ulimits": true, "shm_size": true,
	"read_only": true, "sysctls": true, "userns_mode": true, "cgroup_parent": true,
}

var cosmeticKeys = map[string]bool{
	"container_name": true, "hostname": true, "platform": true, "logging": true, "links": true,
	"networks": true, "exclude_from_hc": true, "tty": true, "stdin_open": true, "stop_grace_period": true,
	"stop_signal": true, "cap_drop": true, "expose": true, "init": true, "extra_hosts": true,
	"labels": true, "dns": true, "mem_limit": true, "cpus": true, "pull_policy": true,
}

// Convert turns one upstream template into a manifest. It never panics

// on odd input: every problem becomes a Skip with a reason code.

// Convert turns one upstream template into a manifest. Every problem
// becomes a Skip with a reason code; it never panics on odd input.
func Convert(id string, src []byte, opt Options) Output {
	out := Output{ID: id, Source: opt.SourcePath}
	skip := func(code, detail string) Output {
		out.Skip = &Skip{Code: code, Detail: detail}
		return out
	}
	if !idPattern.MatchString(id) {
		return skip(ReasonBadID, "id must be lowercase letters, digits and dashes")
	}
	meta := parseHeader(src)
	if meta["ignore"] == "true" {
		return skip(ReasonUpstreamIgnored, "upstream marks the template ignored")
	}
	var doc map[string]any
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return skip(ReasonParse, firstLine(err.Error()))
	}
	if meta["documentation"] == "" || meta["slogan"] == "" {
		return skip(ReasonMissingMetadata, "documentation or slogan header is missing")
	}
	for _, k := range []string{"secrets", "configs"} {
		if _, ok := doc[k]; ok {
			return skip(ReasonUnsupportedTop, "top-level "+k)
		}
	}
	if vols, ok := doc["volumes"].(map[string]any); ok {
		for name, body := range vols {
			if m, ok := body.(map[string]any); ok && (m["external"] != nil || m["driver_opts"] != nil || m["driver"] != nil) {
				return skip(ReasonUnsupportedVolume, "top-level volume "+name+" sets external or driver options")
			}
		}
	}
	svcs, ok := doc["services"].(map[string]any)
	if !ok || len(svcs) == 0 {
		return skip(ReasonParse, "no services")
	}

	build := func(promote map[string]bool) (*conversion, error) {
		c := &conversion{t: newTranslator(), svcs: svcs, meta: meta, named: map[string]string{}, out: map[string]*outService{}}
		c.t.promote = promote
		c.t.id = id
		return c, c.run()
	}
	c, err := build(nil)
	if err == nil && len(c.t.users) > 0 {
		promote := make(map[string]bool, len(c.t.users))
		for k := range c.t.users {
			promote[k] = true
		}
		c, err = build(promote)
	}
	if err != nil {
		code := ReasonValidationFailed
		var se *skipError
		if errors.As(err, &se) {
			return skip(se.code, se.detail)
		}
		return skip(code, err.Error())
	}
	return c.finish(id, opt, out)
}

type skipError struct{ code, detail string }

func (e *skipError) Error() string { return e.code + ": " + e.detail }

type conversion struct {
	t      *translator
	svcs   map[string]any
	meta   map[string]string
	named  map[string]string
	out    map[string]*outService
	ports  map[string]int
	expose map[string][]int
	hcPort map[string]int
	names  []string
}

func (c *conversion) run() error {
	c.names = make([]string, 0, len(c.svcs))
	for name := range c.svcs {
		c.names = append(c.names, name)
		c.t.services[name] = true
	}
	sort.Strings(c.names)
	c.ports = map[string]int{}
	c.expose = map[string][]int{}
	c.declareDomains()
	for _, name := range c.names {
		body, ok := c.svcs[name].(map[string]any)
		if !ok {
			return &skipError{ReasonParse, "service " + name + " is not a mapping"}
		}
		c.t.svc = name
		svc, err := c.service(name, body)
		if err != nil {
			return err
		}
		if c.t.skip != nil {
			return &skipError{c.t.skip.Code, c.t.skip.Detail}
		}
		c.out[name] = svc
	}
	if c.t.skip != nil {
		return &skipError{c.t.skip.Code, c.t.skip.Detail}
	}
	return c.applyAliases()
}

type envItem struct {
	key string
	val string
	has bool
}

func envItems(v any) []envItem {
	var out []envItem
	switch e := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(e))
		for k := range e {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			out = append(out, envItem{key: k, val: str(e[k]), has: e[k] != nil})
		}
	case []any:
		for _, it := range e {
			s := str(it)
			if k, val, ok := strings.Cut(s, "="); ok {
				out = append(out, envItem{key: k, val: val, has: true})
			} else {
				out = append(out, envItem{key: s})
			}
		}
	}
	return out
}

func (c *conversion) service(name string, body map[string]any) (*outService, error) {
	t := c.t
	svc := &outService{}
	if _, ok := body["build"]; ok {
		return nil, &skipError{ReasonBuildFromSource, "service " + name + " builds from source"}
	}
	keys := make([]string, 0, len(body))
	for k := range body {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		switch {
		case unsupportedKeys[k]:
			return nil, &skipError{ReasonUnsupportedKey, name + " sets " + k}
		case cosmeticKeys[k]:
			if k != "expose" {
				t.note(NoteDroppedKey, k)
			}
		}
	}
	img := str(body["image"])
	if img == "" {
		return nil, &skipError{ReasonParse, "service " + name + " has no image"}
	}
	svc.Image = t.translate(img, locImage)
	if t.skip != nil {
		return nil, nil
	}
	if !imageTagPattern.MatchString(svc.Image) {
		svc.Image += ":latest"
	}
	if strings.HasSuffix(svc.Image, ":latest") {
		t.note(NoteUnpinnedImage, svc.Image)
	}
	switch r := str(body["restart"]); r {
	case "always", "unless-stopped", "on-failure", "no":
		svc.Restart = r
	}
	t.svcEnv = map[string]bool{}
	for _, e := range envItems(body["environment"]) {
		t.svcEnv[e.key] = true
	}
	c.environment(svc, body)
	svc.Command = c.command(body["command"], name)
	svc.Entrypoint = c.command(body["entrypoint"], name)
	c.collectPorts(svc, body)
	svc.Volumes = t.volumes(body["volumes"], c.named)
	svc.DependsOn = body["depends_on"]
	svc.Healthcheck = c.healthcheck(body["healthcheck"])
	passThroughHostAccess(svc, body)
	return svc, nil
}

func (c *conversion) environment(svc *outService, body map[string]any) {
	t := c.t
	env := map[string]string{}
	for _, e := range envItems(body["environment"]) {
		if domainDeclPattern.MatchString(e.key) || (!e.has && strings.HasPrefix(e.key, "SERVICE_")) {
			continue
		}
		if e.has {
			env[envName(e.key)] = t.translate(e.val, locEnv)
			continue
		}
		env[envName(e.key)] = t.translate("${"+e.key+"}", locEnv)
	}
	if len(env) > 0 {
		svc.Environment = env
	}
}

func (c *conversion) collectPorts(svc *outService, body map[string]any) {
	t := c.t
	name := t.svc
	var raw []any
	if l, ok := body["ports"].([]any); ok {
		raw = append(raw, l...)
	}
	for _, it := range raw {
		var s string
		if m, ok := it.(map[string]any); ok {
			s = str(m["target"])
			if p := str(m["published"]); p != "" {
				s = p + ":" + s
			}
		} else {
			s = str(it)
		}
		translated := t.translate(s, locOther)
		if strings.HasSuffix(translated, "/udp") {
			t.fail(ReasonUnsupportedProto, "service "+name+" publishes "+translated)
			return
		}
		if parts := strings.Split(translated, ":"); len(parts) >= 2 && strings.Contains(parts[len(parts)-2], "${") {
			translated = parts[len(parts)-1]
			t.note(NoteHostPortPublished, "host port left to the platform for "+translated)
		}
		spec, ok := parsePort(strings.TrimSuffix(translated, "/tcp"))
		if !ok {
			t.fail(ReasonValidationFailed, "service "+name+" port "+s+" is not a plain number")
			return
		}
		c.expose[name] = append(c.expose[name], spec.container)
		if spec.host > 0 {
			t.note(NoteHostPortPublished, spec.String())
		}
		svc.Ports = append(svc.Ports, spec.String())
	}
	if l, ok := body["expose"].([]any); ok {
		for _, it := range l {
			if p, err := strconv.Atoi(strings.Split(str(it), "/")[0]); err == nil {
				c.expose[name] = append(c.expose[name], p)
			}
		}
	}
}

func passThroughHostAccess(svc *outService, body map[string]any) {
	svc.Privileged = body["privileged"] == true
	svc.NetworkMode = str(body["network_mode"])
	svc.Pid = str(body["pid"])
	svc.Ipc = str(body["ipc"])
	svc.CapAdd = strList(body["cap_add"])
	svc.Devices = strList(body["devices"])
	svc.SecurityOpt = strList(body["security_opt"])
	svc.VolumesFrom = strList(body["volumes_from"])
}

func strList(v any) []string {
	l, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(l))
	for _, it := range l {
		out = append(out, str(it))
	}
	return out
}

func (c *conversion) command(v any, svc string) any {
	t := c.t
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		return t.translate(x, locCommand)
	case []any:
		parts := make([]string, 0, len(x))
		aliased := false
		for _, it := range x {
			p := t.translate(str(it), locCommand)
			aliased = aliased || strings.Contains(p, "$GENERATED_")
			parts = append(parts, p)
		}
		if aliased {
			t.note(NoteShellWrapped, "command of "+svc+" runs through sh -c to read a generated secret")
			return []string{"sh", "-c", shellJoin(parts)}
		}
		return parts
	}
	return nil
}

func shellJoin(parts []string) string {
	q := make([]string, len(parts))
	for i, p := range parts {
		if strings.Contains(p, "$GENERATED_") {
			r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`")
			q[i] = `"` + r.Replace(p) + `"`
			continue
		}
		q[i] = "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
	}
	return strings.Join(q, " ")
}

func (c *conversion) healthcheck(v any) map[string]any {
	m, ok := v.(map[string]any)
	if !ok || m["disable"] == true {
		return nil
	}
	t := c.t
	out := map[string]any{}
	for k, val := range m {
		switch k {
		case "test":
			out[k] = c.healthTest(val)
		case "interval", "timeout", "start_period":
			out[k] = str(val)
		case "retries":
			out[k] = val
		}
	}
	if out["test"] == nil {
		t.note(NoteDroppedKey, "healthcheck without a test")
		return nil
	}
	return out
}

func (c *conversion) healthTest(v any) any {
	t := c.t
	switch x := v.(type) {
	case string:
		return []string{"CMD-SHELL", t.translate(x, locCommand)}
	case []any:
		parts := make([]string, 0, len(x))
		for _, it := range x {
			parts = append(parts, t.translate(str(it), locCommand))
		}
		if len(parts) > 0 && parts[0] == "CMD" {
			for _, p := range parts[1:] {
				if strings.Contains(p, "$GENERATED_") {
					return []string{"CMD-SHELL", shellJoin(parts[1:])}
				}
			}
		}
		return parts
	}
	return nil
}

func (c *conversion) applyAliases() error {
	for svc, aliases := range c.t.aliases {
		o := c.out[svc]
		if o == nil {
			continue
		}
		if o.Environment == nil {
			o.Environment = map[string]string{}
		}
		for k, tok := range aliases {
			o.Environment[k] = tok
			c.t.note(NoteGeneratedAlias, svc+": "+k)
		}
	}
	return nil
}

// headerPortService finds the service that serves the upstream

// header's web port when the template declares no address variable.

// httpHealthcheck swaps a bare /dev/tcp connect check on a web service

// for an HTTP request the platform runs as a real readiness probe.
