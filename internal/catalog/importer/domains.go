package importer

import (
	"sort"
	"strconv"
	"strings"
)

func (c *conversion) declareDomains() {
	for _, name := range c.names {
		body, _ := c.svcs[name].(map[string]any)
		for _, e := range envItems(body["environment"]) {
			m := domainDeclPattern.FindStringSubmatch(e.key)
			if m == nil {
				continue
			}
			key := m[2]
			if pm := portSuffix.FindString(key); pm != "" {
				p, _ := strconv.Atoi(strings.TrimPrefix(pm, "_"))
				if _, set := c.ports[name]; !set {
					c.ports[name] = p
				}
				key = strings.TrimSuffix(key, pm)
			}
			c.t.domainKeys[normKey(key)] = name
			c.t.domainSvcs[name] = true
		}
	}
}

func (c *conversion) domainPorts() (map[string]int, error) {
	out := map[string]int{}
	domainSvcs := make([]string, 0, len(c.t.domainSvcs))
	for s := range c.t.domainSvcs {
		domainSvcs = append(domainSvcs, s)
	}
	headerPort, _ := strconv.Atoi(c.meta["port"])
	if len(domainSvcs) == 0 && headerPort > 0 {
		if svc := c.headerPortService(headerPort); svc != "" {
			domainSvcs = append(domainSvcs, svc)
			c.t.domainSvcs[svc] = true
		}
	}
	sort.Strings(domainSvcs)
	for _, s := range domainSvcs {
		switch {
		case c.ports[s] > 0:
			out[s] = c.ports[s]
		case headerPort > 0 && (len(domainSvcs) == 1 || contains(c.expose[s], headerPort)):
			out[s] = headerPort
		case len(c.expose[s]) > 0:
			out[s] = c.expose[s][0]
		default:
			return nil, &skipError{ReasonNoDomainPort, "service " + s + " has no known web port"}
		}
	}
	return out, nil
}

func (c *conversion) headerPortService(port int) string {
	var found []string
	for _, n := range c.names {
		if contains(c.expose[n], port) {
			found = append(found, n)
		}
	}
	if len(found) == 0 && len(c.names) == 1 {
		return c.names[0]
	}
	if len(found) == 1 {
		return found[0]
	}
	return ""
}

func contains(l []int, v int) bool {
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}

func (c *conversion) httpHealthcheck(svc string, port int) {
	hc := c.out[svc].Healthcheck
	if hc == nil {
		c.out[svc].Healthcheck = map[string]any{
			"test":     []string{"CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:" + strconv.Itoa(port) + "/"},
			"interval": "30s", "timeout": "10s", "retries": 5, "start_period": "60s",
		}
		c.t.notes = append(c.t.notes, Note{Code: NoteHealthcheckAdded, Service: svc, Detail: "HTTP probe on port " + strconv.Itoa(port)})
		return
	}
	test, ok := hc["test"].([]string)
	if !ok || !strings.Contains(strings.Join(test, " "), "/dev/tcp/") {
		return
	}
	hc["test"] = []string{"CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:" + strconv.Itoa(port) + "/"}
	c.t.notes = append(c.t.notes, Note{Code: NoteTCPToHTTP, Service: svc, Detail: "TCP connect check became an HTTP probe on port " + strconv.Itoa(port)})
}
