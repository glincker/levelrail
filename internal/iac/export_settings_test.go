package iac

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

func TestExportSettings(t *testing.T) {
	f := newFake()
	live := addSettings(f)
	f.route("GET", `/api/v1/settings/runtime`, func(_ []string, _ map[string]any) (any, error) {
		var all []*runtimesettings.Setting
		for _, s := range live {
			all = append(all, s)
		}
		return map[string]any{"settings": all}, nil
	})
	applyAll(t, f, settingDoc(runtimesettings.KeyHeartbeatJobAttempts, "value: 5"), Options{})
	var secret string
	for _, d := range runtimesettings.Definitions() {
		if d.Sensitive {
			secret = d.Key
			live[d.Key].Source = runtimesettings.SourceSaved
			live[d.Key].Value = json.RawMessage(`"hunter2"`)
			break
		}
	}
	out, err := Export(context.Background(), f, ExportOptions{Settings: true})
	if err != nil {
		t.Fatal(err)
	}
	j := out.Join()
	if len(out.Files) != 1 || !strings.Contains(j, "kind: Setting") || !strings.Contains(j, "value: 5") || strings.Contains(j, "hunter2") {
		t.Fatalf("settings export:\n%s", j)
	}
	if secret != "" && len(out.Warnings) != 1 {
		t.Fatalf("a saved sensitive key must warn once: %v", out.Warnings)
	}
	if p := planFor(t, f, j, Options{}); quiet(p) != 0 {
		t.Fatalf("export does not replan clean: %+v", p.Changes)
	}
}
