package importer

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/compose"
)

type location int

const (
	locEnv location = iota
	locCommand
	locImage
	locOther
)

var (
	braceVarPattern = regexp.MustCompile(`\$\{([^{}]*)\}`)
	bareVarPattern  = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)
	escapedBraced   = regexp.MustCompile(`\$\$\{([A-Za-z_][A-Za-z0-9_]*)\}([A-Za-z0-9_]?)`)
	varNamePattern  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*`)
	portSuffix      = regexp.MustCompile(`_[0-9]+$`)
	promptKeyOK     = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	ownSecretName   = regexp.MustCompile(`(?i)(PASSWORD|PASSWD|SECRET|PASSPHRASE)`)
	thirdPartyName  = regexp.MustCompile(`(?i)(SMTP|MAIL|API|OAUTH|CLIENT|S3|AWS|GITHUB|GITLAB|GOOGLE|STRIPE|OPENAI|SLACK|DISCORD|LDAP|WEBHOOK|SSO|OIDC|SENTRY|TWILIO|ADMIN_EMAIL)`)
	databaseName    = regexp.MustCompile(`(?i)(_DATABASE|_DB|_DB_NAME|_DBNAME)$`)
	secretKeyName   = regexp.MustCompile(`(?i)(PASSWORD|SECRET|TOKEN|KEY|PASS|CREDENTIAL)`)
)

// generatedVar is one generated secret shared by every reference to the
// same kind and key, like the upstream convention.
type generatedVar struct {
	kind   string
	key    string
	length int
	prompt string
}

// userVar is a plain ${VAR} the operator fills in.
type userVar struct {
	key      string
	required bool
	hint     string
	def      string
}

type translator struct {
	svc        string
	skip       *Skip
	notes      []Note
	gens       map[string]*generatedVar
	genOrder   []string
	users      map[string]*userVar
	userOrder  []string
	promptKeys map[string]string
	domainKeys map[string]string
	services   map[string]bool
	aliases    map[string]map[string]string
	domainSvcs map[string]bool
	usedAlias  bool
	svcEnv     map[string]bool
	promote    map[string]bool
	id         string
}

func newTranslator() *translator {
	return &translator{
		gens:       map[string]*generatedVar{},
		users:      map[string]*userVar{},
		promptKeys: map[string]string{},
		domainKeys: map[string]string{},
		services:   map[string]bool{},
		aliases:    map[string]map[string]string{},
		domainSvcs: map[string]bool{},
	}
}

func (t *translator) fail(code, detail string) {
	if t.skip == nil {
		t.skip = &Skip{Code: code, Detail: detail}
	}
}

func (t *translator) note(code, detail string) {
	t.notes = append(t.notes, Note{Code: code, Service: t.svc, Detail: detail})
}

func restoreHoles(s string, holes []string) string {
	for i := len(holes) - 1; i >= 0; i-- {
		s = strings.ReplaceAll(s, "\x01"+strconv.Itoa(i)+"\x02", holes[i])
	}
	return s
}

// translate rewrites every upstream variable in s into this platform's
// forms. Replaced tokens are parked behind sentinels so the nested
// pass never reprocesses its own output.
func (t *translator) translate(s string, loc location) string {
	if !strings.Contains(s, "$") {
		return s
	}
	var holes []string
	hole := func(lit string) string {
		holes = append(holes, lit)
		return "\x01" + strconv.Itoa(len(holes)-1) + "\x02"
	}
	s = escapedBraced.ReplaceAllStringFunc(s, func(m string) string {
		sub := escapedBraced.FindStringSubmatch(m)
		if sub[2] != "" {
			return m
		}
		return hole("$" + sub[1])
	})
	s = strings.ReplaceAll(s, "$$", hole("$"))
	for i := 0; i < 64; i++ {
		if m := braceVarPattern.FindStringSubmatchIndex(s); m != nil {
			s = s[:m[0]] + t.expand(s[m[2]:m[3]], loc, hole) + s[m[1]:]
			continue
		}
		if m := bareVarPattern.FindStringSubmatchIndex(s); m != nil {
			s = s[:m[0]] + t.expand(s[m[2]:m[3]], loc, hole) + s[m[1]:]
			continue
		}
		break
	}
	return restoreHoles(s, holes)
}

// expand resolves one variable body such as NAME, NAME:-default or
// SERVICE_PASSWORD_64_DB.
func (t *translator) expand(body string, loc location, hole func(string) string) string {
	name := varNamePattern.FindString(body)
	if name == "" {
		return hole("${" + body + "}")
	}
	op, arg := splitOperator(body[len(name):])
	if op == "" && len(body) > len(name) {
		return hole("${" + body + "}")
	}
	if loc == locCommand && op == "" && t.svcEnv[name] {
		return hole("$" + envName(name))
	}
	if strings.HasPrefix(name, "SERVICE_") {
		return t.expandMagic(name, op, arg, loc, hole)
	}
	switch op {
	case ":-", "-":
		if t.promote[name] && loc != locImage {
			return t.userPrompt(name, false, "", arg, loc, hole)
		}
		if arg != "" {
			t.note(NoteInlinedDefault, name+"="+arg)
		}
		return arg
	case ":+", "+":
		return arg
	case ":?", "?":
		return t.userPrompt(name, true, arg, "", loc, hole)
	default:
		return t.userPrompt(name, false, "", "", loc, hole)
	}
}

// envName renames environment variables that would collide with the
// platform's own SERVICE_ token namespace.
func envName(k string) string {
	if strings.HasPrefix(k, "SERVICE_") {
		return "GEN_" + strings.TrimPrefix(k, "SERVICE_")
	}
	return k
}

func splitOperator(rest string) (op, arg string) {
	for _, o := range []string{":-", ":?", ":+", "-", "?", "+"} {
		if strings.HasPrefix(rest, o) {
			return o, rest[len(o):]
		}
	}
	return "", rest
}

func (t *translator) userPrompt(name string, required bool, hint, def string, loc location, hole func(string) string) string {
	if !promptKeyOK.MatchString(name) {
		up := strings.ToUpper(strings.TrimLeft(name, "_"))
		if !strings.Contains(name, "_") || !promptKeyOK.MatchString(up) {
			t.fail(ReasonUnknownMagicVar, "variable "+name+" cannot become a setting")
			return ""
		}
		name = up
	}
	if loc == locImage {
		t.fail(ReasonUnresolvedImage, "image uses ${"+name+"} with no default")
		return ""
	}
	if _, isGen := t.promptKeys[name]; isGen && t.promptKeys[name] == "generated" {
		t.fail(ReasonPromptConflict, name+" is both a generated secret and a setting")
		return ""
	}
	u, ok := t.users[name]
	if !ok {
		u = &userVar{key: name}
		t.users[name] = u
		t.userOrder = append(t.userOrder, name)
		t.promptKeys[name] = "user"
	}
	u.required = u.required || required
	if def != "" && u.def == "" {
		u.def = def
	}
	if hint != "" && u.hint == "" {
		u.hint = hint
	}
	return hole("${" + name + "}")
}

func (t *translator) expandMagic(name, op, arg string, loc location, hole func(string) string) string {
	rest := strings.TrimPrefix(name, "SERVICE_")
	kind, _, _ := strings.Cut(rest, "_")
	switch kind {
	case "URL", "FQDN":
		return t.expandDomain(kind, rest, hole)
	case "PASSWORD", "USER", "BASE64", "REALBASE64", "HEX":
		return t.expandGenerated(name, loc, hole)
	}
	if op == ":-" || op == "-" {
		return arg
	}
	return t.userPrompt(rest, op == ":?" || op == "?", arg, "", loc, hole)
}

func (t *translator) expandDomain(kind, rest string, hole func(string) string) string {
	key := strings.TrimPrefix(rest, kind+"_")
	key = portSuffix.ReplaceAllString(key, "")
	svc := t.domainService(key)
	if svc == "" {
		t.fail(ReasonUnknownDomainKey, "no service owns the address named "+key)
		return ""
	}
	t.domainSvcs[svc] = true
	tokenKind := "URL"
	if kind == "FQDN" {
		tokenKind = "HOST"
	}
	return hole("${SERVICE_" + tokenKind + "_" + upperKey(svc) + "}")
}

func (t *translator) domainService(key string) string {
	norm := normKey(key)
	if svc, ok := t.domainKeys[norm]; ok {
		return svc
	}
	for s := range t.services {
		if normKey(s) == norm {
			return s
		}
	}
	if len(t.domainKeys) == 1 {
		for _, svc := range t.domainKeys {
			return svc
		}
	}
	return ""
}

func normKey(s string) string {
	return strings.ToLower(strings.NewReplacer("_", "-", ".", "-").Replace(s))
}

func upperKey(s string) string {
	return strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(s))
}

func (t *translator) expandGenerated(name string, loc location, hole func(string) string) string {
	mvs := compose.FindMagicVars("${" + name + "}")
	if len(mvs) != 1 {
		t.fail(ReasonUnknownMagicVar, "${"+name+"}")
		return ""
	}
	mv := mvs[0]
	var token string
	if mv.Kind == "REALBASE64" {
		token = "${" + name + "}"
	} else {
		g := t.generated(mv)
		if g == nil {
			return ""
		}
		token = "${" + g.prompt + "}"
		if loc == locCommand {
			return hole(t.alias(g.prompt, token))
		}
		return hole(token)
	}
	if loc == locCommand {
		alias := "GENERATED_" + strings.TrimPrefix(name, "SERVICE_")
		return hole(t.alias(alias, token))
	}
	return hole(token)
}

// alias exposes a generated secret to a command or healthcheck as an
// environment variable the shell can expand, since the platform
// resolves generated values only inside environment entries.
func (t *translator) alias(name, token string) string {
	if t.aliases[t.svc] == nil {
		t.aliases[t.svc] = map[string]string{}
	}
	if !strings.HasPrefix(name, "GENERATED_") {
		name = "GENERATED_" + name
	}
	t.aliases[t.svc][name] = token
	t.usedAlias = true
	return "$" + name
}

func (t *translator) generated(mv compose.MagicVar) *generatedVar {
	id := mv.Kind + ":" + mv.Key
	if g, ok := t.gens[id]; ok {
		if mv.Length > g.length {
			g.length = mv.Length
		}
		return g
	}
	prompt := promptName(mv.Key, mv.Kind)
	if t.promptKeys[prompt] == "user" {
		t.fail(ReasonPromptConflict, prompt+" is both a generated secret and a setting")
		return nil
	}
	g := &generatedVar{kind: mv.Kind, key: mv.Key, length: mv.Length, prompt: prompt}
	t.gens[id] = g
	t.genOrder = append(t.genOrder, id)
	t.promptKeys[prompt] = "generated"
	return g
}

func promptName(key, kind string) string {
	key = strings.ToUpper(key)
	switch {
	case key == "":
		return kind
	case strings.HasSuffix(key, "_"+kind) || key == kind:
		return key
	default:
		return key + "_" + kind
	}
}

var generateKindOf = map[string]string{"PASSWORD": "password", "USER": "user", "BASE64": "base64", "HEX": "hex"}

// prompts builds the manifest's env prompts: generated secrets first,
// then operator settings, each in first-use order.
func (t *translator) prompts() []catalog.EnvPrompt {
	var out []catalog.EnvPrompt
	for _, id := range t.genOrder {
		g := t.gens[id]
		gen := generateKindOf[g.kind]
		length := g.length
		if length == 0 && gen == "" {
			continue
		}
		out = append(out, catalog.EnvPrompt{
			Key:         g.prompt,
			Label:       humanize(g.prompt),
			Description: "Generated when the app is installed.",
			Generate:    gen,
			Length:      length,
			Secret:      gen != "user",
		})
	}
	for _, k := range t.userOrder {
		u := t.users[k]
		p := catalog.EnvPrompt{Key: k, Label: humanize(k), Required: u.required, Secret: secretKeyName.MatchString(k)}
		if u.def == "" && !u.required {
			switch {
			case ownSecretName.MatchString(k) && !thirdPartyName.MatchString(k):
				p.Generate, p.Length, p.Secret = "password", 32, true
				p.Description = "Generated when the app is installed."
				out = append(out, p)
				t.notes = append(t.notes, Note{Code: NoteGeneratedBare, Detail: k})
				continue
			case databaseName.MatchString(k):
				u.def = strings.ReplaceAll(t.id, "-", "_")
			}
		}
		switch {
		case u.def != "":
			p.Description = "Defaults to " + u.def + "."
			p.Default = u.def
		case u.hint != "":
			p.Description = u.hint
		case u.required:
			p.Description = "Required."
		default:
			p.Description = "Optional. Leave empty to use the application default."
		}
		out = append(out, p)
	}
	return out
}

func humanize(key string) string {
	parts := strings.Split(strings.ToLower(key), "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
}

func (t *translator) generatedSummary() []string {
	var out []string
	for _, id := range t.genOrder {
		g := t.gens[id]
		out = append(out, fmt.Sprintf("%s(%s)", g.prompt, strings.ToLower(g.kind)))
	}
	sort.Strings(out)
	return out
}
