package compose

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// UnsupportedKey is one compose key that parsed but has no effect here.
// Service is empty for a top-level key; Key is dotted for nested keys.
type UnsupportedKey struct {
	Service     string
	Key         string
	Alternative string
}

// Message is the operator-facing sentence for a Notice.
func (u UnsupportedKey) Message() string {
	where := "top-level"
	if u.Service != "" {
		where = fmt.Sprintf("service %q", u.Service)
	}
	return fmt.Sprintf("%s: %s: is not supported and was ignored; %s", where, u.Key, u.Alternative)
}

const defaultAlternative = "remove it, or bake the setting into the image; it has no effect on how this service runs"

// alternatives names the documented replacement for the keys users paste most.
var alternatives = map[string]string{
	"extends":                              "inline the extended service's keys into this service; extends is not resolved",
	"env_file":                             "move the values into environment: or the app's env vars; env_file is not read",
	"container_name":                       "containers are named <app>-<service>; remove it",
	"user":                                 "set USER in the image, or wrap the start command with entrypoint:",
	"working_dir":                          "set WORKDIR in the image, or cd inside command:",
	"hostname":                             "services reach each other by service name; remove it",
	"expose":                               "internal-only ports need no declaration; add ports: for the one ingress should route to",
	"stop_grace_period":                    "the reconciler's own stop timeout applies; remove it",
	"stop_signal":                          "the image's default stop signal is used; remove it",
	"tty":                                  "remove it; containers run without a TTY",
	"stdin_open":                           "remove it; containers run without stdin",
	"logging":                              "logs are collected by the platform; read them with the logs commands",
	"links":                                "services reach each other by service name; remove it",
	"external_links":                       "services reach each other by service name; remove it",
	"extra_hosts":                          "use service names for in-app hosts, or bake the hosts entry into the image",
	"dns":                                  "the host's resolver is used; remove it",
	"ulimits":                              "set limits in the image or the host's Docker daemon config",
	"sysctls":                              "set sysctls on the host; per-container sysctls are not applied",
	"read_only":                            "remove it; the root filesystem stays writable",
	"tmpfs":                                "use a named volume at that path instead",
	"platform":                             "build or pull the image for the server's architecture",
	"scale":                                "use deploy.replicas",
	"cap_drop":                             "remove it; capabilities are not changed",
	"build":                                "declare a pre-built image: or use a git deploy with build.type: compose",
	"mem_reservation":                      "remove it; only a hard memory limit is applied",
	"memswap_limit":                        "set the app's swap limit in its resource settings",
	"cpu_shares":                           "remove it; CPU weight is not applied",
	"cpuset":                               "set the app's CPU pinning in its resource settings",
	"healthcheck.disable":                  "remove the healthcheck block instead, or set test: [\"NONE\"]",
	"depends_on.restart":                   "remove it; the reconciler restarts dependents on its own",
	"depends_on.required":                  "remove it; every listed dependency is required",
	"deploy.resources.reservations.cpus":   "remove it; only limits are applied",
	"deploy.resources.reservations.memory": "remove it; only limits are applied",
}

var (
	supportedOnce sync.Once
	supportedSvc  map[string]bool
)

// supportedServiceKeys derives the read keys from rawService's yaml tags, so a
// newly parsed field is supported without touching a second list.
func supportedServiceKeys() map[string]bool {
	supportedOnce.Do(func() {
		supportedSvc = map[string]bool{}
		collectTags(reflect.TypeOf(rawService{}), supportedSvc)
		collectTags(reflect.TypeOf(HostAccess{}), supportedSvc)
	})
	return supportedSvc
}

func collectTags(t reflect.Type, into map[string]bool) {
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("yaml")
		name, _, _ := strings.Cut(tag, ",")
		if name != "" && name != "-" {
			into[name] = true
		}
	}
}

// SupportedServiceKeys lists every service key the parser reads, sorted.
func SupportedServiceKeys() []string {
	keys := make([]string, 0)
	for k := range supportedServiceKeys() {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

var supportedTopLevel = map[string]bool{
	"version": true, "services": true, "x-levelrail-domains": true,
	"networks": true, "secrets": true, "configs": true,
}

func alternativeFor(key string) string {
	if a, ok := alternatives[key]; ok {
		return a
	}
	return defaultAlternative
}

// findUnsupported walks the decoded document (anchors and merge keys already
// resolved) and reports every key the parser does not read. Extension fields
// (x-*) are skipped: the Compose spec defines them as ignorable.
func findUnsupported(data []byte) []UnsupportedKey {
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil
	}
	var out []UnsupportedKey
	for _, k := range sortedKeys(doc) {
		switch {
		case supportedTopLevel[k], strings.HasPrefix(k, "x-"):
		case k == "volumes":
			out = append(out, topLevelVolumes(doc[k])...)
		default:
			out = append(out, UnsupportedKey{Key: k, Alternative: alternativeFor("top." + k)})
		}
	}
	services, _ := doc["services"].(map[string]any)
	for _, name := range sortedKeys(services) {
		svc, _ := services[name].(map[string]any)
		out = append(out, serviceUnsupported(name, svc)...)
	}
	return out
}

func topLevelVolumes(v any) []UnsupportedKey {
	vols, _ := v.(map[string]any)
	var out []UnsupportedKey
	for _, name := range sortedKeys(vols) {
		if body, ok := vols[name].(map[string]any); ok && len(body) > 0 {
			out = append(out, UnsupportedKey{
				Key:         "volumes." + name,
				Alternative: "volume options (driver, external, name) are ignored; named volumes are always created by the platform, scoped to the app",
			})
		}
	}
	return out
}

func serviceUnsupported(name string, svc map[string]any) []UnsupportedKey {
	supported := supportedServiceKeys()
	var out []UnsupportedKey
	add := func(key string) {
		out = append(out, UnsupportedKey{Service: name, Key: key, Alternative: alternativeFor(key)})
	}
	for _, k := range sortedKeys(svc) {
		switch {
		case !supported[k] && !strings.HasPrefix(k, "x-"):
			add(k)
		case k == "healthcheck":
			for _, sub := range subKeys(svc[k], "disable", "start_interval") {
				add("healthcheck." + sub)
			}
		case k == "depends_on":
			for _, sub := range nestedMapKeys(svc[k], "restart", "required") {
				add("depends_on." + sub)
			}
		case k == "ports":
			for _, sub := range listItemKeys(svc[k], "host_ip", "name", "app_protocol") {
				add("ports." + sub)
			}
		case k == "volumes":
			for _, sub := range listItemKeys(svc[k], "bind", "tmpfs", "consistency", "volume", "image") {
				add("volumes." + sub)
			}
		case k == "deploy":
			for _, sub := range deployUnsupported(svc[k]) {
				add(sub)
			}
		}
	}
	return out
}

// deployUnsupported covers the nested resources keys that Deploy.unsupported
// (top-level deploy subkeys, already rejected by Validate) cannot see.
func deployUnsupported(v any) []string {
	deploy, _ := v.(map[string]any)
	res, _ := deploy["resources"].(map[string]any)
	var out []string
	if limits, ok := res["limits"].(map[string]any); ok {
		for _, k := range sortedKeys(limits) {
			if k != "cpus" && k != "memory" {
				out = append(out, "deploy.resources.limits."+k)
			}
		}
	}
	if rsv, ok := res["reservations"].(map[string]any); ok {
		for _, k := range sortedKeys(rsv) {
			if k != "devices" {
				out = append(out, "deploy.resources.reservations."+k)
			}
		}
	}
	return out
}

func subKeys(v any, names ...string) []string {
	m, _ := v.(map[string]any)
	var out []string
	for _, n := range names {
		if _, ok := m[n]; ok {
			out = append(out, n)
		}
	}
	return out
}

// nestedMapKeys looks for names inside each value of a map of maps (depends_on long form).
func nestedMapKeys(v any, names ...string) []string {
	m, _ := v.(map[string]any)
	seen := map[string]bool{}
	for _, dep := range sortedKeys(m) {
		for _, n := range subKeys(m[dep], names...) {
			seen[n] = true
		}
	}
	return sortedSet(seen)
}

// listItemKeys looks for names inside each mapping entry of a list (long-form ports and volumes).
func listItemKeys(v any, names ...string) []string {
	list, _ := v.([]any)
	seen := map[string]bool{}
	for _, item := range list {
		for _, n := range subKeys(item, names...) {
			seen[n] = true
		}
	}
	return sortedSet(seen)
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedSet(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
