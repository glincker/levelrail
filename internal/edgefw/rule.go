package edgefw

import (
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

// Action is what a matching rule does.
type Action string

// Rule actions. Allow ends evaluation and skips every later check, deny and
// challenge end it, log and rate_limit let the request continue unless the
// rate limit trips.
const (
	ActionAllow     Action = "allow"
	ActionDeny      Action = "deny"
	ActionChallenge Action = "challenge"
	ActionLog       Action = "log"
	ActionRateLimit Action = "rate_limit"
)

// Field is the request attribute a condition reads.
type Field string

// Condition fields.
const (
	FieldPath      Field = "path"
	FieldHost      Field = "host"
	FieldMethod    Field = "method"
	FieldHeader    Field = "header"
	FieldQuery     Field = "query"
	FieldUserAgent Field = "user_agent"
	FieldIP        Field = "ip"
	FieldCountry   Field = "country"
)

// Op is how a condition compares its field.
type Op string

// Condition operators.
const (
	OpEquals     Op = "equals"
	OpStartsWith Op = "starts_with"
	OpContains   Op = "contains"
	OpGlob       Op = "glob"
	OpInCIDR     Op = "in_cidr"
	OpExists     Op = "exists"
)

// FieldOps lists the operators each field accepts; it is the closed catalog
// the dashboard builder and the validator share.
var FieldOps = map[Field][]Op{
	FieldPath:      {OpEquals, OpStartsWith, OpContains, OpGlob},
	FieldHost:      {OpEquals, OpContains, OpGlob},
	FieldMethod:    {OpEquals},
	FieldHeader:    {OpEquals, OpStartsWith, OpContains, OpGlob, OpExists},
	FieldQuery:     {OpEquals, OpStartsWith, OpContains, OpExists},
	FieldUserAgent: {OpEquals, OpStartsWith, OpContains, OpGlob},
	FieldIP:        {OpInCIDR},
	FieldCountry:   {OpEquals},
}

// Condition is one comparison; a rule matches when all its conditions do.
type Condition struct {
	Field Field  `json:"field"`
	Op    Op     `json:"op"`
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

// Rule is one ordered firewall rule. Scope is a host or empty for every
// host; Position orders rules, lowest first.
type Rule struct {
	ID                string      `json:"id"`
	Name              string      `json:"name"`
	Note              string      `json:"note,omitempty"`
	Scope             string      `json:"scope,omitempty"`
	Enabled           bool        `json:"enabled"`
	Position          int         `json:"position"`
	Conditions        []Condition `json:"conditions"`
	Action            Action      `json:"action"`
	RateCount         int         `json:"rate_count,omitempty"`
	RateWindowSeconds int         `json:"rate_window_seconds,omitempty"`
	CreatedAt         string      `json:"created_at,omitempty"`
	UpdatedAt         string      `json:"updated_at,omitempty"`
}

// IPList reports whether the rule is a plain IP list entry (one ip
// condition) and which action list it belongs to.
func (r Rule) IPList() (Action, bool) {
	if len(r.Conditions) == 1 && r.Conditions[0].Field == FieldIP && (r.Action == ActionDeny || r.Action == ActionAllow) {
		return r.Action, true
	}
	return "", false
}

// FieldError names one invalid field with plain text.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationError carries every problem found in a rule or setting.
type ValidationError struct{ Fields []FieldError }

func (e *ValidationError) Error() string {
	if len(e.Fields) == 0 {
		return "firewall: invalid"
	}
	parts := make([]string, len(e.Fields))
	for i, f := range e.Fields {
		parts[i] = f.Field + ": " + f.Message
	}
	return "firewall: " + strings.Join(parts, "; ")
}

type verrs struct{ list []FieldError }

func (v *verrs) add(field, format string, a ...any) {
	v.list = append(v.list, FieldError{Field: field, Message: fmt.Sprintf(format, a...)})
}

func (v *verrs) err() error {
	if len(v.list) == 0 {
		return nil
	}
	return &ValidationError{Fields: v.list}
}

const (
	maxNameLen   = 80
	maxNoteLen   = 200
	maxValueLen  = 256
	maxNameField = 64
	maxConds     = 8
	maxWildcards = 8
	maxRateCount = 1000000
	maxRateWin   = 3600
)

// Normalize trims and canonicalizes a rule in place before validation.
func Normalize(r *Rule) {
	r.Name = strings.TrimSpace(r.Name)
	r.Note = strings.TrimSpace(r.Note)
	r.Scope = strings.ToLower(strings.TrimSpace(r.Scope))
	for i := range r.Conditions {
		c := &r.Conditions[i]
		c.Name = strings.TrimSpace(c.Name)
		c.Value = strings.TrimSpace(c.Value)
		switch c.Field {
		case FieldMethod, FieldCountry:
			c.Value = strings.ToUpper(c.Value)
		case FieldHost:
			c.Value = strings.ToLower(c.Value)
		case FieldHeader:
			c.Name = strings.ToLower(c.Name)
		}
	}
}

// ValidateRule checks a normalized rule against the limits.
func ValidateRule(r *Rule) error {
	v := &verrs{}
	if r.Name == "" || len(r.Name) > maxNameLen {
		v.add("name", "use 1 to %d characters", maxNameLen)
	}
	if len(r.Note) > maxNoteLen {
		v.add("note", "at most %d characters", maxNoteLen)
	}
	if r.Scope != "" && !validHost(r.Scope) {
		v.add("scope", "must be a host name such as app.example.com")
	}
	switch r.Action {
	case ActionAllow, ActionDeny, ActionChallenge, ActionLog:
		if r.RateCount != 0 || r.RateWindowSeconds != 0 {
			v.add("action", "rate settings only apply to the rate limit action")
		}
	case ActionRateLimit:
		if r.RateCount < 1 || r.RateCount > maxRateCount {
			v.add("rate_count", "use 1 to %d requests", maxRateCount)
		}
		if r.RateWindowSeconds < 1 || r.RateWindowSeconds > maxRateWin {
			v.add("rate_window_seconds", "use 1 to %d seconds", maxRateWin)
		}
	default:
		v.add("action", "choose allow, deny, challenge, log or rate limit")
	}
	if len(r.Conditions) == 0 || len(r.Conditions) > maxConds {
		v.add("conditions", "use 1 to %d conditions", maxConds)
	}
	for i, c := range r.Conditions {
		validateCondition(v, fmt.Sprintf("conditions[%d]", i), c)
	}
	return v.err()
}

func validateCondition(v *verrs, at string, c Condition) {
	ops, ok := FieldOps[c.Field]
	if !ok {
		v.add(at+".field", "unknown field %q", c.Field)
		return
	}
	found := false
	for _, o := range ops {
		if o == c.Op {
			found = true
		}
	}
	if !found {
		v.add(at+".op", "%s does not support %q", c.Field, c.Op)
		return
	}
	needsName := c.Field == FieldHeader || c.Field == FieldQuery
	if needsName && (c.Name == "" || len(c.Name) > maxNameField || !validToken(c.Name)) {
		v.add(at+".name", "enter a name of 1 to %d letters, digits, dashes or underscores", maxNameField)
	}
	if !needsName && c.Name != "" {
		v.add(at+".name", "%s takes no name", c.Field)
	}
	if c.Op == OpExists {
		if c.Value != "" {
			v.add(at+".value", "exists takes no value")
		}
		return
	}
	if c.Value == "" || len(c.Value) > maxValueLen {
		v.add(at+".value", "enter a value of 1 to %d characters", maxValueLen)
		return
	}
	switch {
	case c.Field == FieldIP:
		if _, err := trafficpolicy.ParseIPOrPrefix(c.Value); err != nil {
			v.add(at+".value", "must be an IP address or CIDR range")
		}
	case c.Field == FieldMethod:
		if !validMethod(c.Value) {
			v.add(at+".value", "must be an HTTP method such as GET")
		}
	case c.Field == FieldCountry:
		if !trafficpolicy.IsCountryCode(c.Value) {
			v.add(at+".value", "must be a two letter country code")
		}
	case c.Field == FieldPath && (c.Op == OpEquals || c.Op == OpStartsWith) && !strings.HasPrefix(c.Value, "/"):
		v.add(at+".value", "a path starts with /")
	case c.Op == OpGlob && strings.Count(c.Value, "*")+strings.Count(c.Value, "?") > maxWildcards:
		v.add(at+".value", "at most %d wildcards", maxWildcards)
	}
}

func validToken(s string) bool {
	for _, r := range s {
		letter := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
		if !letter && (r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

func validMethod(s string) bool {
	switch s {
	case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "CONNECT", "TRACE":
		return true
	}
	return false
}

func validHost(s string) bool {
	if len(s) > 253 || s == "" {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '.' {
			return false
		}
	}
	return !strings.HasPrefix(s, ".") && !strings.HasSuffix(s, ".")
}

// IPRule builds the canonical rule for an IP list entry.
func IPRule(action Action, ip, scope, note string) (Rule, error) {
	p, err := trafficpolicy.ParseIPOrPrefix(strings.TrimSpace(ip))
	if err != nil {
		return Rule{}, &ValidationError{Fields: []FieldError{{Field: "ip", Message: "must be an IP address or CIDR range"}}}
	}
	name := "Block " + p.String()
	if action == ActionAllow {
		name = "Allow " + p.String()
	}
	r := Rule{Name: name, Note: note, Scope: scope, Enabled: true, Action: action,
		Conditions: []Condition{{Field: FieldIP, Op: OpInCIDR, Value: p.String()}}}
	Normalize(&r)
	return r, ValidateRule(&r)
}
