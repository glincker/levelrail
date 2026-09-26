package iac

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

type fakeRoute struct {
	method string
	re     *regexp.Regexp
	h      func(m []string, body map[string]any) (any, error)
}

// fakeCP is an in-memory control plane speaking just the endpoints iac uses.
type fakeCP struct {
	seq       int
	projects  map[string]map[string]any
	envs      map[string]map[string]any
	envVars   map[string]map[string]any
	tags      map[string]map[string]any
	dbs       map[string]map[string]any
	apps      map[string]map[string]any
	secrets   map[string]map[string]bool
	lbs       map[string]map[string]any
	pipelines map[string]map[string]map[string]any
	alerts    map[string]map[string]map[string]any
	channels  []map[string]any
	deny      func(method, path string) bool
	writes    []string
	routes    []fakeRoute
}

func newFake() *fakeCP {
	f := &fakeCP{projects: map[string]map[string]any{}, envs: map[string]map[string]any{}, envVars: map[string]map[string]any{}, tags: map[string]map[string]any{},
		dbs: map[string]map[string]any{}, apps: map[string]map[string]any{}, secrets: map[string]map[string]bool{}, lbs: map[string]map[string]any{},
		pipelines: map[string]map[string]map[string]any{}, alerts: map[string]map[string]map[string]any{}}
	f.channels = []map[string]any{{"id": "ch1", "name": "ops"}}
	f.routes = f.buildRoutes()
	return f
}

func (f *fakeCP) id(prefix string) string {
	f.seq++
	return fmt.Sprintf("%s%d", prefix, f.seq)
}

func list(m map[string]map[string]any) []any {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]any, 0, len(keys))
	for _, k := range keys {
		out = append(out, m[k])
	}
	return out
}

func (f *fakeCP) Do(_ context.Context, method, path string, body, out any) error {
	if f.deny != nil && f.deny(method, path) {
		return &StatusError{Status: http.StatusForbidden, Message: "forbidden"}
	}
	var b map[string]any
	if body != nil {
		raw, _ := json.Marshal(body)
		_ = json.Unmarshal(raw, &b)
	}
	if method != http.MethodGet {
		f.writes = append(f.writes, method+" "+path)
	}
	for _, r := range f.routes {
		if r.method != method {
			continue
		}
		m := r.re.FindStringSubmatch(path)
		if m == nil {
			continue
		}
		res, err := r.h(m, b)
		if err != nil {
			return err
		}
		if out != nil && res != nil {
			raw, _ := json.Marshal(res)
			return json.Unmarshal(raw, out)
		}
		return nil
	}
	return &StatusError{Status: http.StatusNotFound, Message: "no route " + method + " " + path}
}

func (f *fakeCP) route(method, pattern string, h func(m []string, body map[string]any) (any, error)) {
	f.routes = append(f.routes, fakeRoute{method: method, re: regexp.MustCompile("^" + pattern + "$"), h: h})
}

var errNotFound = &StatusError{Status: http.StatusNotFound, Message: "not found"}

func (f *fakeCP) buildRoutes() []fakeRoute {
	f.routes = nil
	f.route("GET", `/api/v1/projects`, func(_ []string, _ map[string]any) (any, error) { return list(f.projects), nil })
	f.route("POST", `/api/v1/projects`, func(_ []string, b map[string]any) (any, error) {
		p := map[string]any{"id": f.id("p"), "name": b["name"]}
		f.projects[p["id"].(string)] = p
		return p, nil
	})
	f.route("GET", `/api/v1/projects/([^/]+)/env`, func(m []string, _ map[string]any) (any, error) { return f.envVars["p/"+m[1]], nil })
	f.route("PUT", `/api/v1/projects/([^/]+)/env`, func(m []string, b map[string]any) (any, error) { f.envVars["p/"+m[1]] = b; return b, nil })
	f.route("GET", `/api/v1/projects/([^/]+)/environments`, func(m []string, _ map[string]any) (any, error) {
		out := []any{}
		for _, e := range list(f.envs) {
			if e.(map[string]any)["project_id"] == m[1] {
				out = append(out, e)
			}
		}
		return out, nil
	})
	f.route("POST", `/api/v1/projects/([^/]+)/environments`, func(m []string, b map[string]any) (any, error) {
		e := map[string]any{"id": f.id("e"), "project_id": m[1], "name": b["name"], "protected": b["protected"] == true}
		f.envs[e["id"].(string)] = e
		return e, nil
	})
	f.route("PATCH", `/api/v1/environments/([^/]+)`, func(m []string, b map[string]any) (any, error) {
		f.envs[m[1]]["protected"] = b["protected"]
		return f.envs[m[1]], nil
	})
	f.route("GET", `/api/v1/environments/([^/]+)/env`, func(m []string, _ map[string]any) (any, error) { return f.envVars["e/"+m[1]], nil })
	f.route("PUT", `/api/v1/environments/([^/]+)/env`, func(m []string, b map[string]any) (any, error) { f.envVars["e/"+m[1]] = b; return b, nil })
	f.route("GET", `/api/v1/tags`, func(_ []string, _ map[string]any) (any, error) { return list(f.tags), nil })
	f.route("POST", `/api/v1/tags`, func(_ []string, b map[string]any) (any, error) { return f.ensureTag(b["name"].(string)), nil })
	f.route("GET", `/api/v1/tags/([^/]+)/apps`, func(m []string, _ map[string]any) (any, error) {
		name := f.tags[m[1]]["name"]
		out := []any{}
		for _, n := range sortedKeys(f.apps) {
			for _, t := range stringList(f.apps[n]["tags"]) {
				if t == name {
					out = append(out, map[string]any{"name": n})
				}
			}
		}
		return out, nil
	})
	f.route("GET", `/api/v1/databases`, func(_ []string, _ map[string]any) (any, error) { return list(f.dbs), nil })
	f.route("POST", `/api/v1/databases`, func(_ []string, b map[string]any) (any, error) { f.dbs[b["name"].(string)] = b; return b, nil })
	f.route("PUT", `/api/v1/databases/([^/]+)/project`, func(m []string, b map[string]any) (any, error) {
		f.dbs[m[1]]["project_id"] = b["project_id"]
		return f.dbs[m[1]], nil
	})
	f.appRoutes()
	f.extraRoutes()
	return f.routes
}

func (f *fakeCP) ensureTag(name string) map[string]any {
	for _, t := range f.tags {
		if t["name"] == name {
			return t
		}
	}
	t := map[string]any{"id": f.id("t"), "name": name}
	f.tags[t["id"].(string)] = t
	return t
}

func (f *fakeCP) appRoutes() {
	f.route("GET", `/api/v1/apps`, func(_ []string, _ map[string]any) (any, error) { return list(f.apps), nil })
	f.route("POST", `/api/v1/apps`, func(_ []string, b map[string]any) (any, error) {
		name := b["name"].(string)
		if _, dup := f.apps[name]; dup {
			return nil, &StatusError{Status: http.StatusConflict, Message: "exists"}
		}
		b["env_dirty"] = false
		f.apps[name] = b
		f.secrets[name] = map[string]bool{}
		for _, s := range stringList(b["secret_env"]) {
			f.secrets[name][s] = false
		}
		return b, nil
	})
	f.route("GET", `/api/v1/apps/([^/]+)`, func(m []string, _ map[string]any) (any, error) {
		a, ok := f.apps[m[1]]
		if !ok {
			return nil, errNotFound
		}
		return a, nil
	})
	f.route("PUT", `/api/v1/apps/([^/]+)`, func(m []string, b map[string]any) (any, error) {
		old, ok := f.apps[m[1]]
		if !ok {
			return nil, errNotFound
		}
		for _, keep := range []string{"project_id", "environment_id", "tags", "vault_env", "volumes"} {
			if _, sent := b[keep]; !sent && old[keep] != nil {
				b[keep] = old[keep]
			}
		}
		if fmt.Sprint(old["env"]) != fmt.Sprint(b["env"]) {
			b["env_dirty"] = true
		}
		f.apps[m[1]] = b
		return b, nil
	})
	f.route("DELETE", `/api/v1/apps/([^/]+)`, func(m []string, _ map[string]any) (any, error) { delete(f.apps, m[1]); return nil, nil })
	f.route("GET", `/api/v1/apps/([^/]+)/secrets`, func(m []string, _ map[string]any) (any, error) {
		out := []any{}
		for _, k := range sortedKeys(f.secrets[m[1]]) {
			if f.secrets[m[1]][k] {
				out = append(out, map[string]any{"key": k})
			}
		}
		return out, nil
	})
	f.route("PUT", `/api/v1/apps/([^/]+)/secrets/([^/]+)`, func(m []string, _ map[string]any) (any, error) {
		f.secrets[m[1]][m[2]] = true
		return nil, nil
	})
	f.route("POST", `/api/v1/apps/([^/]+)/restart`, func(_ []string, _ map[string]any) (any, error) { return nil, nil })
	f.route("PUT", `/api/v1/apps/([^/]+)/project`, func(m []string, b map[string]any) (any, error) {
		f.apps[m[1]]["project_id"] = b["project_id"]
		return nil, nil
	})
	f.route("PUT", `/api/v1/apps/([^/]+)/environment`, func(m []string, b map[string]any) (any, error) {
		f.apps[m[1]]["environment_id"] = b["environment_id"]
		return nil, nil
	})
	f.route("POST", `/api/v1/apps/([^/]+)/tags`, func(m []string, b map[string]any) (any, error) {
		t := f.ensureTag(b["name"].(string))
		f.apps[m[1]]["tags"] = append(stringAny(f.apps[m[1]]["tags"]), t["name"])
		return t, nil
	})
	f.route("DELETE", `/api/v1/apps/([^/]+)/tags/([^/]+)`, func(m []string, _ map[string]any) (any, error) {
		name := f.tags[m[2]]["name"]
		var keep []any
		for _, t := range stringAny(f.apps[m[1]]["tags"]) {
			if t != name {
				keep = append(keep, t)
			}
		}
		f.apps[m[1]]["tags"] = keep
		return nil, nil
	})
}

func stringAny(v any) []any {
	l, _ := v.([]any)
	return l
}

func (f *fakeCP) extraRoutes() {
	f.route("GET", `/api/v1/apps/([^/]+)/loadbalancer`, func(m []string, _ map[string]any) (any, error) {
		if c, ok := f.lbs[m[1]]; ok {
			return map[string]any{"configured": true, "config": c}, nil
		}
		return map[string]any{"configured": false}, nil
	})
	f.route("PUT", `/api/v1/apps/([^/]+)/loadbalancer`, func(m []string, b map[string]any) (any, error) { f.lbs[m[1]] = b; return nil, nil })
	f.route("DELETE", `/api/v1/apps/([^/]+)/loadbalancer`, func(m []string, _ map[string]any) (any, error) { delete(f.lbs, m[1]); return nil, nil })
	f.route("GET", `/api/v1/apps/([^/]+)/pipelines`, func(m []string, _ map[string]any) (any, error) { return list(f.pipelines[m[1]]), nil })
	f.route("POST", `/api/v1/apps/([^/]+)/pipelines`, func(m []string, b map[string]any) (any, error) {
		if f.pipelines[m[1]] == nil {
			f.pipelines[m[1]] = map[string]map[string]any{}
		}
		b["source"] = "ui"
		f.pipelines[m[1]][b["name"].(string)] = b
		return b, nil
	})
	f.route("PUT", `/api/v1/apps/([^/]+)/pipelines/([^/]+)`, func(m []string, b map[string]any) (any, error) {
		b["source"] = "ui"
		f.pipelines[m[1]][m[2]] = b
		return b, nil
	})
	f.route("GET", `/api/v1/apps/([^/]+)/alerts`, func(m []string, _ map[string]any) (any, error) { return list(f.alerts[m[1]]), nil })
	f.route("POST", `/api/v1/apps/([^/]+)/alerts`, func(m []string, b map[string]any) (any, error) {
		if f.alerts[m[1]] == nil {
			f.alerts[m[1]] = map[string]map[string]any{}
		}
		b["id"] = f.id("a")
		f.alerts[m[1]][b["name"].(string)] = b
		return b, nil
	})
	f.route("PUT", `/api/v1/apps/([^/]+)/alerts/([^/]+)`, func(m []string, b map[string]any) (any, error) {
		b["id"] = m[2]
		f.alerts[m[1]][b["name"].(string)] = b
		return b, nil
	})
	f.route("GET", `/api/v1/notification-channels`, func(_ []string, _ map[string]any) (any, error) { return f.channels, nil })
}

func (f *fakeCP) writeCount() int { return len(f.writes) }

func hasWrite(f *fakeCP, prefix string) bool {
	for _, w := range f.writes {
		if strings.HasPrefix(w, prefix) {
			return true
		}
	}
	return false
}
