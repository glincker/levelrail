package alerting

import (
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/changes"
)

func TestAlertLinksAndSuffix(t *testing.T) {
	until := time.Date(2026, 1, 2, 14, 2, 0, 0, time.UTC)
	res := changes.Result{Since: until.Add(-30 * time.Minute), Until: until, Changes: []changes.Change{{Kind: changes.KindDeploy}}}
	logs, deploy := alertLinks("https://d.example/apps/web/alerts", res)
	if !strings.HasPrefix(logs, "https://d.example/apps/web/logs?") || !strings.Contains(logs, "from=2026-01-02T13%3A32%3A00Z") || deploy != "https://d.example/apps/web/deploys" {
		t.Fatalf("links = %q %q", logs, deploy)
	}
	if l, d := alertLinks("https://d.example/x", res); l != "" || d != "" {
		t.Fatalf("unexpected links for a non alerts link: %q %q", l, d)
	}
	ev := Event{LogsLink: logs, DeployLink: deploy}
	if s := linksSuffix(ev); !strings.Contains(s, "Logs for this window: "+logs) || !strings.Contains(s, "Nearest deploy: "+deploy) {
		t.Fatalf("suffix = %q", s)
	}
	ev.Resolved = true
	if linksSuffix(ev) != "" {
		t.Fatal("resolved events carry no links")
	}
}

func TestRenderMessageTemplate(t *testing.T) {
	v := 5.0
	ev := Event{Rule: Rule{Name: "5xx", ResourceID: "service:blog", LastValue: &v}, LogsLink: "https://d.example/apps/blog/logs"}
	got := RenderMessageTemplate("{{app}} is {{status}} ({{rule}}={{value}}) {{url}} {{nope}}", alertTemplateVars(ev))
	if want := "blog is firing (5xx=5) https://d.example/apps/blog/logs {{nope}}"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	ev.CustomText = got
	if summaryBody(ev) != got {
		t.Fatalf("custom text must replace the default body, got %q", summaryBody(ev))
	}
	d := DeployOutcome{AppName: "blog", Image: "blog:1", Succeeded: true, Author: "ana", Commit: "abc123"}
	if out := RenderMessageTemplate("{{app}} {{status}} by {{author}} at {{commit}}", deployTemplateVars(d)); out != "blog succeeded by ana at abc123" {
		t.Fatalf("deploy render = %q", out)
	}
}
