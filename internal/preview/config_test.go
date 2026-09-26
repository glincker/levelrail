package preview

import (
	"io"
	"log/slog"
	"testing"
	"time"
)

func lookupFrom(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestConfigFromEnv_Defaults(t *testing.T) {
	c := ConfigFromEnv(lookupFrom(nil), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if c.Enabled {
		t.Error("previews must default to off")
	}
	if c.Image != DefaultImage || c.Timeout != 30*time.Second || c.MemoryMB != 512 || c.CPUs != 1 {
		t.Errorf("unexpected defaults: %+v", c)
	}
	if c.ViewportW != 1280 || c.ViewportH != 800 || c.ThumbWidth != 640 || c.Quality != 72 {
		t.Errorf("unexpected image defaults: %+v", c)
	}
	if c.KeepPerApp != 5 || c.TTL != 30*24*time.Hour || c.MaxTotalBytes != 200<<20 || c.ImageTTL != 14*24*time.Hour {
		t.Errorf("unexpected retention defaults: %+v", c)
	}
	if c.MinFreeRAM != 768<<20 || c.MinFreeDisk != 2048<<20 || c.QueueDepth != 8 {
		t.Errorf("unexpected gate defaults: %+v", c)
	}
}

func TestConfigFromEnv_Overrides(t *testing.T) {
	c := ConfigFromEnv(lookupFrom(map[string]string{
		EnvEnabled: "true", EnvKeepPerApp: "9", EnvViewport: "1024x768", EnvTimeout: "45s",
		EnvMinFreeRAMMB: "1024", EnvMaxTotalMB: "50", EnvCPUs: "0.5",
	}), nil)
	if !c.Enabled || c.KeepPerApp != 9 || c.ViewportW != 1024 || c.ViewportH != 768 || c.Timeout != 45*time.Second {
		t.Errorf("overrides not applied: %+v", c)
	}
	if c.MinFreeRAM != 1024<<20 || c.MaxTotalBytes != 50<<20 || c.CPUs != 0.5 {
		t.Errorf("overrides not applied: %+v", c)
	}
}

func TestConfigFromEnv_MalformedFallsBack(t *testing.T) {
	c := ConfigFromEnv(lookupFrom(map[string]string{
		EnvKeepPerApp: "many", EnvViewport: "wide", EnvTimeout: "soon", EnvEnabled: "maybe", EnvMemoryMB: "-4",
	}), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if c.KeepPerApp != 5 || c.ViewportW != 1280 || c.Timeout != 30*time.Second || c.Enabled || c.MemoryMB != 512 {
		t.Errorf("malformed values should fall back: %+v", c)
	}
}
