package templateupgrade

import (
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/compose"
)

const mergeBase = "services:\n  web:\n    image: app:1.0.0\n    environment:\n      MODE: base\n      KEEP: base\n"
const mergeTpl = "services:\n  web:\n    image: app:1.1.0\n    environment:\n      MODE: new\n      KEEP: new\n      ADDED: yes\n"

func planFor(t *testing.T, installed, tpl string) Plan {
	t.Helper()
	i, err := compose.Parse([]byte(installed))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := compose.Parse([]byte(mergeBase))
	n, err := compose.Parse([]byte(tpl))
	if err != nil {
		t.Fatal(err)
	}
	return Build(Input{AppID: "a", TemplateID: "t", Installed: i, Baseline: b, Template: n})
}

func TestMergeCarriesUserChoices(t *testing.T) {
	installed := "services:\n  web:\n    image: app:1.0.5\n    environment:\n      MODE: base\n      KEEP: mine\n      OWN: 1\n"
	out, err := Merge(installed, mergeTpl, planFor(t, installed, mergeTpl))
	if err != nil {
		t.Fatal(err)
	}
	f, err := compose.Parse([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	w := f.Services["web"]
	if w.Image != "app:1.0.5" {
		t.Errorf("pinned image lost: %s", w.Image)
	}
	if w.Environment["KEEP"] != "mine" || w.Environment["OWN"] != "1" {
		t.Errorf("user env lost: %v", w.Environment)
	}
	if w.Environment["MODE"] != "new" || w.Environment["ADDED"] == "" {
		t.Errorf("template env not applied: %v", w.Environment)
	}
}

func TestMergeUnpinnedTakesTemplateImage(t *testing.T) {
	out, err := Merge(mergeBase, mergeTpl, planFor(t, mergeBase, mergeTpl))
	if err != nil {
		t.Fatal(err)
	}
	f, _ := compose.Parse([]byte(out))
	if f.Services["web"].Image != "app:1.1.0" {
		t.Errorf("image: %s", f.Services["web"].Image)
	}
}

func TestMergeRefusesUserService(t *testing.T) {
	installed := mergeBase + "  sidecar:\n    image: x:1\n"
	_, err := Merge(installed, mergeTpl, planFor(t, installed, mergeTpl))
	if !errors.Is(err, ErrUserServices) {
		t.Fatalf("want ErrUserServices, got %v", err)
	}
}
