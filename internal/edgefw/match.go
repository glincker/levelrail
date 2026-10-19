package edgefw

import (
	"net/http"
	"net/netip"
	"strings"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

// maxUA caps how much of a user agent any matcher reads.
const maxUA = 512

// Request is the slice of an HTTP request the firewall reads. Host carries
// no port; Country is empty when no geo source is configured.
type Request struct {
	Method     string
	Host       string
	Path       string
	RawQuery   string
	UserAgent  string
	Header     http.Header
	IP         netip.Addr
	Country    string
	PassCookie string
}

type matcher struct {
	field  Field
	op     Op
	name   string
	val    string
	lower  string
	prefix netip.Prefix
}

func compileCondition(c Condition) matcher {
	m := matcher{field: c.Field, op: c.Op, name: c.Name, val: c.Value, lower: strings.ToLower(c.Value)}
	if c.Field == FieldIP {
		m.prefix, _ = trafficpolicy.ParseIPOrPrefix(c.Value)
	}
	return m
}

func (m *matcher) match(r *Request) bool {
	switch m.field {
	case FieldIP:
		return r.IP.IsValid() && m.prefix.Contains(r.IP)
	case FieldMethod:
		return r.Method == m.val
	case FieldCountry:
		return r.Country != "" && r.Country == m.val
	case FieldPath:
		return textMatch(m.op, r.Path, m.val, m.lower, false)
	case FieldHost:
		return textMatch(m.op, r.Host, m.val, m.lower, true)
	case FieldUserAgent:
		ua := r.UserAgent
		if len(ua) > maxUA {
			ua = ua[:maxUA]
		}
		return textMatch(m.op, ua, m.val, m.lower, true)
	case FieldHeader:
		vals := r.Header.Values(m.name)
		if m.op == OpExists {
			return len(vals) > 0
		}
		for _, v := range vals {
			if textMatch(m.op, v, m.val, m.lower, true) {
				return true
			}
		}
		return false
	case FieldQuery:
		return queryMatch(m, r.RawQuery)
	}
	return false
}

func textMatch(op Op, s, val, lower string, fold bool) bool {
	switch op {
	case OpEquals:
		if fold {
			return strings.EqualFold(s, val)
		}
		return s == val
	case OpStartsWith:
		if fold {
			return len(s) >= len(val) && strings.EqualFold(s[:len(val)], val)
		}
		return strings.HasPrefix(s, val)
	case OpContains:
		if fold {
			return containsFold(s, lower)
		}
		return strings.Contains(s, val)
	case OpGlob:
		if fold {
			return globMatch(lower, s, true)
		}
		return globMatch(val, s, false)
	}
	return false
}

// containsFold reports whether s contains lowerSub ignoring ASCII case,
// without allocating.
func containsFold(s, lowerSub string) bool {
	n := len(lowerSub)
	if n == 0 {
		return true
	}
	for i := 0; i+n <= len(s); i++ {
		j := 0
		for ; j < n; j++ {
			c := s[i+j]
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c != lowerSub[j] {
				break
			}
		}
		if j == n {
			return true
		}
	}
	return false
}

// globMatch matches s against pattern where * is any run (slashes included)
// and ? is one byte. pattern must already be lower case when fold is true.
func globMatch(pattern, s string, fold bool) bool {
	p, i := 0, 0
	star, mark := -1, 0
	for i < len(s) {
		if p < len(pattern) && pattern[p] == '*' {
			star, mark = p, i
			p++
			continue
		}
		if p < len(pattern) && (pattern[p] == '?' || eqByte(pattern[p], s[i], fold)) {
			p++
			i++
			continue
		}
		if star < 0 {
			return false
		}
		mark++
		i, p = mark, star+1
	}
	for p < len(pattern) && pattern[p] == '*' {
		p++
	}
	return p == len(pattern)
}

func eqByte(p, c byte, fold bool) bool {
	if fold && c >= 'A' && c <= 'Z' {
		c += 'a' - 'A'
	}
	return p == c
}

// queryMatch scans the raw query for the named parameter without parsing
// or decoding it, so no allocation happens and values are compared as sent.
func queryMatch(m *matcher, raw string) bool {
	for raw != "" {
		var pair string
		pair, raw, _ = strings.Cut(raw, "&")
		k, v, has := strings.Cut(pair, "=")
		if k != m.name {
			continue
		}
		if m.op == OpExists {
			return true
		}
		if !has {
			v = ""
		}
		if textMatch(m.op, v, m.val, m.lower, false) {
			return true
		}
	}
	return false
}

type compiledRule struct {
	id        string
	name      string
	scope     string
	action    Action
	conds     []matcher
	rateCount int
	rateWin   int
}

func compileRule(r Rule) *compiledRule {
	c := &compiledRule{id: r.ID, name: r.Name, scope: r.Scope, action: r.Action, rateCount: r.RateCount, rateWin: r.RateWindowSeconds}
	c.conds = make([]matcher, len(r.Conditions))
	for i, cond := range r.Conditions {
		c.conds[i] = compileCondition(cond)
	}
	return c
}

func (c *compiledRule) matches(r *Request) bool {
	for i := range c.conds {
		if !c.conds[i].match(r) {
			return false
		}
	}
	return true
}
