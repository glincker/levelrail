package runtimesettings

import "context"

// Keys for the deploy preview render tier.
const (
	KeyPreviewDefaultMode    = "preview.default_mode"
	KeyPreviewMaxConcurrent  = "preview.max_concurrent"
	KeyPreviewMinInterval    = "preview.min_interval_seconds"
	KeyPreviewRenderTimeout  = "preview.render_timeout_seconds"
	KeyPreviewIdleStop       = "preview.idle_stop_seconds"
	KeyPreviewBrowserMemory  = "preview.browser_memory_mb"
	KeyPreviewBrowserCPUMill = "preview.browser_cpu_millis"
)

// PreviewRender is the live render tuning for app card previews.
type PreviewRender struct {
	DefaultMode   string
	MaxConcurrent int
	MinInterval   int
	RenderTimeout int
	IdleStop      int
	MemoryMB      int
	CPUMillis     int
}

// PreviewRender returns the current values; a change applies to the next render.
func (s *Service) PreviewRender(ctx context.Context) PreviewRender {
	return PreviewRender{
		DefaultMode:   s.String(ctx, KeyPreviewDefaultMode),
		MaxConcurrent: s.Int(ctx, KeyPreviewMaxConcurrent),
		MinInterval:   s.Int(ctx, KeyPreviewMinInterval),
		RenderTimeout: s.Int(ctx, KeyPreviewRenderTimeout),
		IdleStop:      s.Int(ctx, KeyPreviewIdleStop),
		MemoryMB:      s.Int(ctx, KeyPreviewBrowserMemory),
		CPUMillis:     s.Int(ctx, KeyPreviewBrowserCPUMill),
	}
}

func init() {
	Register(Definition{
		Key: KeyPreviewDefaultMode, Category: CategoryDeploys, Kind: KindString, Default: "screenshot",
		Env: "APP_PREVIEW_DEFAULT_MODE", Apply: ApplyLive, AllowedStrings: []string{"metadata", "screenshot"},
		Title:       "Default app preview mode",
		Description: "How app cards get their picture when an app has no preview choice of its own. screenshot renders the real page once with a short-lived headless browser and caches the image. metadata only reads the page's own share image or title and never starts a browser. A per-app choice always wins.",
	})
	Register(Definition{
		Key: KeyPreviewMaxConcurrent, Category: CategoryDeploys, Kind: KindInt, Default: "1",
		Env: "APP_PREVIEW_MAX_CONCURRENT", Apply: ApplyLive, CheckValue: intRangeCheck(1, 8),
		Title:       "Preview renders at once",
		Description: "How many app pages may be rendered at the same time, from 1 to 8. Each render uses one headless browser, so keep 1 on small hosts.",
	})
	Register(Definition{
		Key: KeyPreviewMinInterval, Category: CategoryDeploys, Kind: KindInt, Default: "3600",
		Env: "APP_PREVIEW_MIN_INTERVAL_SECONDS", Apply: ApplyLive, CheckValue: intRangeCheck(0, 604800),
		Title:       "Least seconds between renders of one app",
		Description: "An app is not rendered again sooner than this, so a burst of redeploys costs one render. 0 turns the limit off. A manual recapture ignores it.",
	})
	Register(Definition{
		Key: KeyPreviewRenderTimeout, Category: CategoryDeploys, Kind: KindInt, Default: "30",
		Env: "APP_PREVIEW_RENDER_TIMEOUT_SECONDS", Apply: ApplyLive, CheckValue: intRangeCheck(5, 300),
		Title:       "Preview render time limit",
		Description: "Seconds one page render may take, browser start included, from 5 to 300. A slow page is recorded as failed, never retried in a loop.",
	})
	Register(Definition{
		Key: KeyPreviewIdleStop, Category: CategoryDeploys, Kind: KindInt, Default: "120",
		Env: "APP_PREVIEW_IDLE_STOP_SECONDS", Apply: ApplyLive, CheckValue: intRangeCheck(0, 3600),
		Title:       "Stop the preview browser after idle seconds",
		Description: "The headless browser is started on demand and stopped once no render has needed it for this long, so it holds no memory while idle. 0 stops it right after each render.",
	})
	Register(Definition{
		Key: KeyPreviewBrowserMemory, Category: CategoryDeploys, Kind: KindInt, Default: "512",
		Env: "APP_PREVIEW_MEMORY_MB", Apply: ApplyLive, CheckValue: intRangeCheck(128, 8192),
		Title:       "Preview browser memory limit",
		Description: "Memory cap in MB for the headless browser container, from 128 to 8192. A change applies to the next browser start.",
	})
	Register(Definition{
		Key: KeyPreviewBrowserCPUMill, Category: CategoryDeploys, Kind: KindInt, Default: "1000",
		Env: "APP_PREVIEW_BROWSER_CPU_MILLIS", Apply: ApplyLive, CheckValue: intRangeCheck(100, 8000),
		Title:       "Preview browser CPU limit",
		Description: "CPU cap for the headless browser container in thousandths of a core (1000 is one core), from 100 to 8000. A change applies to the next browser start.",
	})
}
