package api

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

func TestRuntimeSettingPreviewCacheMode(t *testing.T) {
	for _, def := range runtimesettings.Definitions() {
		if def.Key == build.KeyPreviewCacheMode {
			if def.Default != build.PreviewCacheIsolated || def.Env != build.EnvPreviewCacheMode {
				t.Fatalf("unexpected definition: %+v", def)
			}
			return
		}
	}
	t.Fatal("setting not registered")
}
