package catalog

import (
	"strings"
	"testing"
)

// The platform never unescapes `$$` in template compose text, so `sh -c`
// expands it to its PID and silently breaks the intended variable.
var dollarEscapeAllowlist = map[string]string{}

type dollarHit struct {
	Service string
	Line    int
	Text    string
}

func findDollarEscapes(compose string) []dollarHit {
	var hits []dollarHit
	service := ""
	inServices := false
	svcIndent := 0
	for i, line := range strings.Split(compose, "\n") {
		trimmed := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " "))
		switch {
		case indent == 0 && trimmed != "":
			inServices = trimmed == "services:"
			service = ""
		case inServices && indent > 0 && svcIndent == 0:
			svcIndent = indent
			service = strings.TrimSuffix(trimmed, ":")
		case inServices && indent == svcIndent && strings.HasSuffix(trimmed, ":"):
			service = strings.TrimSuffix(trimmed, ":")
		}
		if strings.Contains(line, "$$") {
			hits = append(hits, dollarHit{Service: service, Line: i + 1, Text: trimmed})
		}
	}
	return hits
}

func TestDollarEscape_CheckerCatchesOldMosquittoLine(t *testing.T) {
	old := "services:\n  mosquitto:\n    command: [\"sh\", \"-c\", \"mosquitto_passwd -b /f user \\\"$$MQTT_PASSWORD\\\"\"]\n"
	hits := findDollarEscapes(old)
	if len(hits) != 1 || hits[0].Service != "mosquitto" || hits[0].Line != 3 {
		t.Fatalf("hits = %+v, want one hit on mosquitto line 3", hits)
	}
}

func TestDollarEscape_NoTemplateUsesDoubleDollar(t *testing.T) {
	for _, tpl := range Templates {
		if _, ok := dollarEscapeAllowlist[tpl.ID]; ok {
			continue
		}
		for _, h := range findDollarEscapes(tpl.Compose) {
			t.Errorf("template %q service %q line %d uses $$ (not unescaped by the platform, use $VAR): %s", tpl.ID, h.Service, h.Line, h.Text)
		}
	}
}
