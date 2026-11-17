package catalog

import (
	"strings"
	"testing"
)

// `$$` is Compose's escape: the engine hands the container a literal `$`
// (command, entrypoint, healthcheck, environment). A template that needs a
// container-side variable in `sh -c` must use it, and it is never resolved
// as a magic variable.

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

func TestDollarEscape_DoubleDollarIsLegalAndUnescaped(t *testing.T) {
	for _, id := range []string{"novu", "argilla", "automatisch", "zitadel"} {
		tpl, ok := TemplateByID(id)
		if !ok {
			t.Fatalf("%s not served", id)
		}
		if id != "zitadel" && len(findDollarEscapes(tpl.Compose)) == 0 {
			t.Errorf("%s no longer uses $$; the escape test lost its coverage", id)
		}
	}
	// The compiled catalog predates the escape: none of those may carry one.
	for _, tpl := range Templates {
		if len(findDollarEscapes(tpl.Compose)) == 0 {
			continue
		}
		switch tpl.ID {
		case "novu", "argilla", "automatisch":
		default:
			t.Errorf("template %q unexpectedly uses $$", tpl.ID)
		}
	}
}
