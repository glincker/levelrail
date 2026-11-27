package api

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

func TestPlatformPreflightTemplate(t *testing.T) {
	rt := &Router{}
	host := runtime.GOOS + "/" + runtime.GOARCH
	if reason, _ := rt.platformPreflight(context.Background(), catalog.Template{ID: "x", Platforms: []string{host}}); reason != "" {
		t.Errorf("matching platform refused: %s", reason)
	}
	if reason, _ := rt.platformPreflight(context.Background(), catalog.Template{ID: "x"}); reason != "" {
		t.Errorf("unchecked platforms refused: %s", reason)
	}
	reason, overridable := rt.platformPreflight(context.Background(), catalog.Template{ID: "x", Platforms: []string{"plan9/mips"}})
	if !overridable || !strings.Contains(reason, "plan9/mips") || !strings.Contains(reason, host) {
		t.Errorf("mismatch reason = %q overridable=%v", reason, overridable)
	}
}
