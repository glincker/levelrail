package iac

func namesOf(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func fieldDNSNames(f map[string]any) []string {
	m, _ := f["dns"].(map[string]any)
	return namesOf(m["names"])
}

// checkAppDNS fails the plan when a declared dns name is already owned by another app.
func (p *planner) checkAppDNS(r *Resource, f map[string]any) bool {
	names := fieldDNSNames(f)
	if len(names) == 0 {
		return true
	}
	live := map[string][]string{}
	for n, a := range p.st.apps {
		live[n] = a.wire.DNSNames
	}
	declared := map[string][]string{}
	for _, o := range p.res {
		if o.Kind == KindApp && o != r {
			declared[o.Name] = fieldDNSNames(o.Fields)
			delete(live, o.Name)
		}
	}
	delete(live, r.Name)
	if err := dnsConflict(r.Name, names, live, declared); err != nil {
		p.failCode(r, CodeDNSNameConflict, "%s", err.Error())
		return false
	}
	return true
}
