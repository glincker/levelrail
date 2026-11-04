package iac

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// addSettings serves the runtime settings routes from the real registry. Each
// key starts at its default; PUT saves and DELETE resets.
func addSettings(f *fakeCP) map[string]*runtimesettings.Setting {
	live := map[string]*runtimesettings.Setting{}
	for _, d := range runtimesettings.Definitions() {
		dflt, _ := json.Marshal(d.Default)
		if d.Kind == runtimesettings.KindBool {
			dflt = []byte(d.Default)
		}
		live[d.Key] = &runtimesettings.Setting{Key: d.Key, Kind: string(d.Kind), Default: dflt, Value: dflt, Source: runtimesettings.SourceDefault,
			Apply: string(d.Apply), Risk: string(d.Risk), Sensitive: d.Sensitive}
	}
	get := func(m []string) (*runtimesettings.Setting, error) {
		s, ok := live[m[1]]
		if !ok {
			return nil, &StatusError{Status: http.StatusNotFound, Message: "unknown setting"}
		}
		return s, nil
	}
	f.route("GET", `/api/v1/settings/runtime/([^/]+)`, func(m []string, _ map[string]any) (any, error) { return get(m) })
	f.route("PUT", `/api/v1/settings/runtime/([^/]+)`, func(m []string, b map[string]any) (any, error) {
		s, err := get(m)
		if err != nil {
			return nil, err
		}
		if b["confirm_weakening"] != nil {
			return nil, &StatusError{Status: http.StatusBadRequest, Message: "file applies must never confirm"}
		}
		s.Value, _ = json.Marshal(b["value"])
		s.Source = runtimesettings.SourceSaved
		return s, nil
	})
	f.route("DELETE", `/api/v1/settings/runtime/([^/]+)`, func(m []string, _ map[string]any) (any, error) {
		s, err := get(m)
		if err != nil {
			return nil, err
		}
		s.Value, s.Source = s.Default, runtimesettings.SourceDefault
		return s, nil
	})
	return live
}

func settingDoc(key, spec string) string {
	return "version: 1\nkind: Setting\nmetadata:\n  name: " + key + "\nspec:\n  " + spec + "\n"
}

func TestPlanSettingSaveCurrentThenNoop(t *testing.T) {
	f := newFake()
	live := addSettings(f)
	doc := settingDoc(runtimesettings.KeyHeartbeatJobAttempts, "value: 3")
	p := planFor(t, f, doc, Options{})
	if p.Summary.Update != 1 || !strings.Contains(p.Changes[0].Reason, "save the current value") {
		t.Fatalf("an unsaved equal value must plan a save: %+v", p.Changes)
	}
	if out := applyAll(t, f, doc, Options{}); out.Applied != 1 || out.Failed != 0 {
		t.Fatalf("apply: %+v", out)
	}
	if live[runtimesettings.KeyHeartbeatJobAttempts].Source != runtimesettings.SourceSaved {
		t.Fatal("apply did not save the row")
	}
	if p := planFor(t, f, doc, Options{}); quiet(p) != 0 || len(p.Changes[0].Warnings) != 0 {
		t.Fatalf("replan not clean: %+v", p.Changes)
	}
}

func TestPlanSettingUpdateAndValidation(t *testing.T) {
	f := newFake()
	addSettings(f)
	p := planFor(t, f, settingDoc(runtimesettings.KeyHeartbeatJobAttempts, "value: 5"), Options{})
	if p.Summary.Update != 1 || p.Changes[0].Fields[0].Old != "3" || p.Changes[0].Fields[0].New != "5" {
		t.Fatalf("update: %+v", p.Changes)
	}
	p = planFor(t, f, settingDoc(runtimesettings.KeyHeartbeatJobAttempts, "value: 500"), Options{})
	if p.Summary.Error != 1 {
		t.Fatalf("an out of range value must be a plan error: %+v", p.Changes)
	}
	p = planFor(t, f, settingDoc("no.such_key", "value: 1"), Options{})
	if p.Summary.Error != 1 || !strings.Contains(p.Changes[0].Reason, "unknown setting key") {
		t.Fatalf("unknown key: %+v", p.Changes)
	}
}

func TestPlanSettingRefusesSensitiveAndWeakening(t *testing.T) {
	f := newFake()
	addSettings(f)
	p := planFor(t, f, settingDoc(runtimesettings.KeyAuditAnchorWebhookSecret, `value: "x"`), Options{})
	if p.Summary.Error != 1 || !strings.Contains(p.Changes[0].Reason, "credential") || len(p.Changes[0].Fields) != 0 {
		t.Fatalf("sensitive key: %+v", p.Changes)
	}
	doc := settingDoc(runtimesettings.KeyAuditSensitiveReads, "value: false")
	p = planFor(t, f, doc, Options{})
	if p.Summary.Error != 1 || !strings.Contains(p.Changes[0].Reason, "allow_weaken") {
		t.Fatalf("weakening: %+v", p.Changes)
	}
	if out := applyAll(t, f, doc, Options{}); out.Applied != 0 {
		t.Fatalf("a weakening apply must change nothing: %+v", out)
	}
	for _, w := range f.writes {
		if strings.Contains(w, "/settings/") {
			t.Fatalf("unexpected settings write %s", w)
		}
	}
}

func TestPlanSettingRestartNoteIsAWarning(t *testing.T) {
	f := newFake()
	live := addSettings(f)
	doc := settingDoc(runtimesettings.KeyTrustLoopbackForwarded, "value: true")
	p := planFor(t, f, doc, Options{})
	if p.Summary.Update != 1 || len(p.Changes[0].Warnings) != 1 || !strings.Contains(p.Changes[0].Warnings[0], "restart") {
		t.Fatalf("restart note: %+v", p.Changes)
	}
	applyAll(t, f, doc, Options{})
	live[runtimesettings.KeyTrustLoopbackForwarded].PendingRestart = true
	p = planFor(t, f, doc, Options{})
	if quiet(p) != 0 || p.Summary.Error != 0 || len(p.Changes[0].Warnings) != 1 {
		t.Fatalf("pending restart is a warning, not drift: %+v", p.Changes)
	}
}

func TestPlanSettingReset(t *testing.T) {
	f := newFake()
	live := addSettings(f)
	reset := settingDoc(runtimesettings.KeyHeartbeatJobAttempts, "reset: true")
	if p := planFor(t, f, reset, Options{}); quiet(p) != 0 {
		t.Fatalf("reset of an unsaved key is a noop: %+v", p.Changes)
	}
	applyAll(t, f, settingDoc(runtimesettings.KeyHeartbeatJobAttempts, "value: 7"), Options{})
	if out := applyAll(t, f, reset, Options{}); out.Applied != 1 {
		t.Fatalf("reset apply: %+v", out)
	}
	if live[runtimesettings.KeyHeartbeatJobAttempts].Source != runtimesettings.SourceDefault {
		t.Fatal("reset did not delete the row")
	}

	s := live[runtimesettings.KeyAuditSensitiveReads]
	s.Value, s.Source = json.RawMessage("true"), runtimesettings.SourceSaved
	s.Default = json.RawMessage("false")
	p := planFor(t, f, settingDoc(runtimesettings.KeyAuditSensitiveReads, "reset: true"), Options{})
	if p.Summary.Error != 1 || !strings.Contains(p.Changes[0].Reason, "allow_weaken") {
		t.Fatalf("a reset to a weaker default must be refused: %+v", p.Changes)
	}
}

func TestPlanSettingSpecNeedsValueOrReset(t *testing.T) {
	for _, spec := range []string{"reset: false", "{value: 1, reset: true}"} {
		docs, issues := ParseDocuments([]Source{{Name: "s.yaml", Data: []byte(settingDoc(runtimesettings.KeyHeartbeatJobAttempts, spec))}})
		if len(issues) == 0 {
			_, issues = Build(docs, Options{})
		}
		if len(issues) == 0 {
			t.Fatalf("spec %q must be rejected", spec)
		}
	}
}
