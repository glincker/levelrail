package preview

import (
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// Environment variables ConfigFromEnv reads.
const (
	EnvEnabled       = "APP_PREVIEW_ENABLED"
	EnvImage         = "APP_PREVIEW_IMAGE"
	EnvTimeout       = "APP_PREVIEW_TIMEOUT"
	EnvPullTimeout   = "APP_PREVIEW_PULL_TIMEOUT"
	EnvMemoryMB      = "APP_PREVIEW_MEMORY_MB"
	EnvCPUs          = "APP_PREVIEW_CPUS"
	EnvViewport      = "APP_PREVIEW_VIEWPORT"
	EnvThumbWidth    = "APP_PREVIEW_THUMB_WIDTH"
	EnvQuality       = "APP_PREVIEW_QUALITY"
	EnvMaxThumbKB    = "APP_PREVIEW_MAX_THUMB_KB"
	EnvBlankRatio    = "APP_PREVIEW_BLANK_RATIO"
	EnvKeepPerApp    = "APP_PREVIEW_KEEP_PER_APP"
	EnvTTLDays       = "APP_PREVIEW_TTL_DAYS"
	EnvMaxTotalMB    = "APP_PREVIEW_MAX_TOTAL_MB"
	EnvImageTTLDays  = "APP_PREVIEW_IMAGE_TTL_DAYS"
	EnvMinFreeRAMMB  = "APP_PREVIEW_MIN_FREE_RAM_MB"
	EnvMinFreeDiskMB = "APP_PREVIEW_MIN_FREE_DISK_MB"
	EnvQueueDepth    = "APP_PREVIEW_QUEUE_DEPTH"
	EnvSweepInterval = "APP_PREVIEW_SWEEP_INTERVAL"
	EnvDefaultMode   = "APP_PREVIEW_DEFAULT_MODE"
	EnvThumbHeight   = "APP_PREVIEW_THUMB_HEIGHT"
	EnvMetaTimeout   = "APP_PREVIEW_META_TIMEOUT"
	EnvMetaMaxHTMLKB = "APP_PREVIEW_META_MAX_HTML_KB"
	EnvMetaMaxImgKB  = "APP_PREVIEW_META_MAX_IMAGE_KB"
	EnvMetaRedirects = "APP_PREVIEW_META_MAX_REDIRECTS"
	EnvMetaMinImgPx  = "APP_PREVIEW_META_MIN_IMAGE_PX"
)

// DefaultImage is the pinned capture browser; override with APP_PREVIEW_IMAGE.
const DefaultImage = "docker.io/chromedp/headless-shell:151.0.7922.109"

// Config holds every preview threshold. ConfigFromEnv fills defaults, so a
// Manager never sees a zero value it has to interpret.
type Config struct {
	Enabled       bool
	Image         string
	Timeout       time.Duration
	PullTimeout   time.Duration
	MemoryMB      int64
	CPUs          float64
	ViewportW     int
	ViewportH     int
	ThumbWidth    int
	Quality       int
	MaxThumbKB    int
	BlankRatio    float64
	KeepPerApp    int
	TTL           time.Duration
	MaxTotalBytes int64
	ImageTTL      time.Duration
	MinFreeRAM    int64
	MinFreeDisk   int64
	QueueDepth    int
	SweepInterval time.Duration

	DefaultMode  Mode
	ThumbHeight  int
	MetaTimeout  time.Duration
	MetaMaxHTML  int64
	MetaMaxImage int64
	MetaRedirect int
	MetaMinImgPx int
}

// ConfigFromEnv reads Config from lookup (os.LookupEnv in production).
// Unset or malformed values fall back to the defaults.
func ConfigFromEnv(lookup func(string) (string, bool), logger *slog.Logger) Config {
	if logger == nil {
		logger = slog.Default()
	}
	e := envReader{lookup: lookup, logger: logger}
	w, h := e.viewport(EnvViewport, 1280, 800)
	return Config{
		Enabled:       e.boolean(EnvEnabled, true),
		Image:         e.str(EnvImage, DefaultImage),
		Timeout:       e.duration(EnvTimeout, 30*time.Second),
		PullTimeout:   e.duration(EnvPullTimeout, 5*time.Minute),
		MemoryMB:      int64(e.integer(EnvMemoryMB, 512)),
		CPUs:          e.float(EnvCPUs, 1.0),
		ViewportW:     w,
		ViewportH:     h,
		ThumbWidth:    e.integer(EnvThumbWidth, 640),
		Quality:       e.integer(EnvQuality, 72),
		MaxThumbKB:    e.integer(EnvMaxThumbKB, 200),
		BlankRatio:    e.float(EnvBlankRatio, 0.995),
		KeepPerApp:    e.integer(EnvKeepPerApp, 5),
		TTL:           time.Duration(e.integer(EnvTTLDays, 30)) * 24 * time.Hour,
		MaxTotalBytes: int64(e.integer(EnvMaxTotalMB, 200)) << 20,
		ImageTTL:      time.Duration(e.integer(EnvImageTTLDays, 14)) * 24 * time.Hour,
		MinFreeRAM:    int64(e.integer(EnvMinFreeRAMMB, 768)) << 20,
		MinFreeDisk:   int64(e.integer(EnvMinFreeDiskMB, 2048)) << 20,
		QueueDepth:    e.integer(EnvQueueDepth, 8),
		SweepInterval: e.duration(EnvSweepInterval, time.Hour),
		DefaultMode:   e.mode(EnvDefaultMode, ModeMetadata),
		ThumbHeight:   e.integer(EnvThumbHeight, 400),
		MetaTimeout:   e.duration(EnvMetaTimeout, 5*time.Second),
		MetaMaxHTML:   int64(e.integer(EnvMetaMaxHTMLKB, 512)) << 10,
		MetaMaxImage:  int64(e.integer(EnvMetaMaxImgKB, 2048)) << 10,
		MetaRedirect:  e.integer(EnvMetaRedirects, 3),
		MetaMinImgPx:  e.integer(EnvMetaMinImgPx, 120),
	}
}

type envReader struct {
	lookup func(string) (string, bool)
	logger *slog.Logger
}

func (e envReader) raw(key string) (string, bool) {
	v, ok := e.lookup(key)
	v = strings.TrimSpace(v)
	return v, ok && v != ""
}

func (e envReader) bad(key, v string) {
	e.logger.Warn("preview: ignoring malformed setting", slog.String("key", key), slog.String("value", v))
}

func (e envReader) str(key, def string) string {
	if v, ok := e.raw(key); ok {
		return v
	}
	return def
}

func (e envReader) boolean(key string, def bool) bool {
	v, ok := e.raw(key)
	if !ok {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		e.bad(key, v)
		return def
	}
	return b
}

func (e envReader) integer(key string, def int) int {
	v, ok := e.raw(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		e.bad(key, v)
		return def
	}
	return n
}

func (e envReader) float(key string, def float64) float64 {
	v, ok := e.raw(key)
	if !ok {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f <= 0 {
		e.bad(key, v)
		return def
	}
	return f
}

func (e envReader) duration(key string, def time.Duration) time.Duration {
	v, ok := e.raw(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		e.bad(key, v)
		return def
	}
	return d
}

func (e envReader) viewport(key string, defW, defH int) (int, int) {
	v, ok := e.raw(key)
	if !ok {
		return defW, defH
	}
	ws, hs, found := strings.Cut(strings.ToLower(v), "x")
	w, werr := strconv.Atoi(ws)
	h, herr := strconv.Atoi(hs)
	if !found || werr != nil || herr != nil || w < 320 || h < 200 || w > 4096 || h > 4096 {
		e.bad(key, v)
		return defW, defH
	}
	return w, h
}

func (e envReader) mode(key string, def Mode) Mode {
	v, ok := e.raw(key)
	if !ok {
		return def
	}
	m, err := ParseMode(strings.ToLower(v))
	if err != nil {
		e.bad(key, v)
		return def
	}
	return m
}
