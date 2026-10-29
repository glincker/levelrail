package templateupgrade

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/compose"
)

func parse(t *testing.T, body string) *compose.File {
	t.Helper()
	f, err := compose.Parse([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return f
}

// summary renders a plan as stable lines so golden expectations stay readable.
func summary(p Plan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "risk=%s changes=%t automatic=%t\n", p.Risk, p.HasChanges, p.Automatic)
	for _, bl := range p.Blockers {
		fmt.Fprintf(&b, "blocker %s %s\n", bl.Code, bl.Service)
	}
	for _, s := range p.Services {
		fmt.Fprintf(&b, "svc %s %s\n", s.Name, s.Status)
		if s.Image != nil {
			fmt.Fprintf(&b, "  image %s %s -> %s preserved=%t\n", s.Image.Kind, s.Image.From, s.Image.To, s.Image.Preserved)
		}
		for _, e := range s.Env {
			fmt.Fprintf(&b, "  env %s %s generated=%t\n", e.Name, e.Action, e.Generated)
		}
		for _, v := range s.Volumes {
			fmt.Fprintf(&b, "  volume %s %s\n", v.Name, v.Action)
		}
	}
	return b.String()
}

func check(t *testing.T, got Plan, want string) {
	t.Helper()
	if s := summary(got); s != want {
		t.Fatalf("plan mismatch\n--- got\n%s--- want\n%s", s, want)
	}
}

const base1 = `
services:
  app:
    image: acme/app:1.4.2
    environment:
      LOG_LEVEL: info
      LEGACY_FLAG: "1"
      SECRET_KEY: ${SERVICE_PASSWORD_KEY}
    volumes:
      - data:/var/lib/app
`

func TestPatchBumpIsLowRisk(t *testing.T) {
	tpl := strings.Replace(base1, "1.4.2", "1.4.3", 1)
	p := Build(Input{AppID: "a", TemplateID: "acme", Installed: parse(t, base1), Template: parse(t, tpl)})
	check(t, p, "risk=low changes=true automatic=true\nsvc app changed\n  image patch acme/app:1.4.2 -> acme/app:1.4.3 preserved=false\n")
	if p.FromVersion != "app=acme/app:1.4.2" || p.ToVersion != "app=acme/app:1.4.3" {
		t.Fatalf("versions %q %q", p.FromVersion, p.ToVersion)
	}
}

func TestMajorBumpWithRemovedEnvIsHigh(t *testing.T) {
	tpl := `
services:
  app:
    image: acme/app:2.0.0
    environment:
      LOG_LEVEL: info
      SECRET_KEY: ${SERVICE_PASSWORD_KEY}
      NEW_TOKEN: ${SERVICE_PASSWORD_TOKEN}
    volumes:
      - data:/var/lib/app
`
	p := Build(Input{AppID: "a", TemplateID: "acme", Installed: parse(t, base1), Template: parse(t, tpl)})
	check(t, p, "risk=high changes=true automatic=false\nsvc app changed\n  image major acme/app:1.4.2 -> acme/app:2.0.0 preserved=false\n  env LEGACY_FLAG remove generated=false\n  env NEW_TOKEN add generated=true\n")
}

func TestUserEditedEnvIsPreserved(t *testing.T) {
	inst := strings.Replace(base1, "LOG_LEVEL: info", "LOG_LEVEL: debug", 1)
	tpl := strings.Replace(base1, "1.4.2", "1.5.0", 1)
	withBase := Build(Input{AppID: "a", TemplateID: "acme", Installed: parse(t, inst), Baseline: parse(t, base1), Template: parse(t, tpl)})
	check(t, withBase, "risk=low changes=true automatic=true\nsvc app changed\n  image minor acme/app:1.4.2 -> acme/app:1.5.0 preserved=false\n  env LOG_LEVEL keep_user generated=false\n")

	// Without a baseline the differing value is still never overwritten.
	noBase := Build(Input{AppID: "a", TemplateID: "acme", Installed: parse(t, inst), Template: parse(t, tpl)})
	var kept bool
	for _, e := range noBase.Services[0].Env {
		kept = kept || (e.Name == "LOG_LEVEL" && e.Action == EnvKeepUser)
	}
	if !kept {
		t.Fatalf("expected keep_user for LOG_LEVEL: %s", summary(noBase))
	}
}

func TestVolumeRemovalBlocks(t *testing.T) {
	tpl := `
services:
  app:
    image: acme/app:1.4.2
    environment:
      LOG_LEVEL: info
      LEGACY_FLAG: "1"
      SECRET_KEY: ${SERVICE_PASSWORD_KEY}
`
	p := Build(Input{AppID: "a", TemplateID: "acme", Installed: parse(t, base1), Template: parse(t, tpl)})
	check(t, p, "risk=high changes=true automatic=false\nblocker volume_removed app\nsvc app changed\n  volume data remove\n")
}

func TestNewServiceAndStableJSON(t *testing.T) {
	tpl := base1 + `
  worker:
    image: acme/worker:1.0.0
`
	in := Input{AppID: "a", TemplateID: "acme", Installed: parse(t, base1), Template: parse(t, tpl)}
	p := Build(in)
	check(t, p, "risk=high changes=true automatic=false\nsvc app unchanged\nsvc worker added\n  image new  -> acme/worker:1.0.0 preserved=false\n")
	a, _ := json.Marshal(p)
	b, _ := json.Marshal(Build(in))
	if string(a) != string(b) {
		t.Fatal("plan json is not stable")
	}
	if strings.Contains(string(a), "info") {
		t.Fatal("plan must not carry env values")
	}
}

func TestNoChanges(t *testing.T) {
	p := Build(Input{AppID: "a", TemplateID: "acme", Installed: parse(t, base1), Template: parse(t, base1)})
	check(t, p, "risk=none changes=false automatic=false\nsvc app unchanged\n")
}
