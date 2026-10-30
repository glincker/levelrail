package footprint

import (
	"context"
	"log/slog"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

// Env vars read by ApplyGCPercent and RunTrim.
const (
	EnvGCPercent       = "APP_GC_PERCENT"
	EnvTrimInterval    = "APP_MEM_TRIM_INTERVAL"
	EnvTrimMinIdleMB   = "APP_MEM_TRIM_MIN_IDLE_MB"
	EnvTrimBootDelay   = "APP_MEM_TRIM_BOOT_DELAY"
	DefaultTrimBoot    = 12 * time.Second
	DefaultGCPercent   = 50
	DefaultTrimEvery   = 2 * time.Minute
	DefaultTrimIdleMB  = 24
	trimLoopName       = "memory trim"
	bytesPerMB         = 1 << 20
	maxGCPercentAccept = 400
)

// ApplyGCPercent returns the percent it set, or 0 when GOGC already decides. It lowers the GC target below Go's 100 default unless GOGC or APP_GC_PERCENT is set; the live heap is small, so the extra GC cost is negligible.
func ApplyGCPercent(getenv func(string) string) int {
	if getenv == nil {
		getenv = os.Getenv
	}
	if getenv("GOGC") != "" {
		return 0
	}
	pct := DefaultGCPercent
	if v := strings.TrimSpace(getenv(EnvGCPercent)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= maxGCPercentAccept {
			pct = n
		}
	}
	debug.SetGCPercent(pct)
	return pct
}

// RunTrim returns heap the runtime is holding but not using to the OS. Boot allocates transient hundreds of MB (catalog parse, password hashing) and the scavenger gives it back slowly, so RSS stays high for no reason.
func RunTrim(ctx context.Context, logger *slog.Logger, getenv func(string) string) {
	if getenv == nil {
		getenv = os.Getenv
	}
	every := DefaultTrimEvery
	if d, err := time.ParseDuration(strings.TrimSpace(getenv(EnvTrimInterval))); err == nil {
		every = d
	}
	if every <= 0 {
		return
	}
	minIdle := uint64(DefaultTrimIdleMB)
	if n, err := strconv.ParseUint(strings.TrimSpace(getenv(EnvTrimMinIdleMB)), 10, 64); err == nil {
		minIdle = n
	}
	go trimOnceAfterBoot(ctx, getenv)
	_ = Every(ctx, trimLoopName, every, logger, func(context.Context) error {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		if (ms.HeapIdle-ms.HeapReleased)/bytesPerMB < minIdle {
			return nil
		}
		debug.FreeOSMemory()
		return nil
	})
}

// trimOnceAfterBoot releases the boot-time heap shortly after startup instead of waiting a full trim interval; a delay of 0 or less turns it off.
func trimOnceAfterBoot(ctx context.Context, getenv func(string) string) {
	delay := DefaultTrimBoot
	if d, err := time.ParseDuration(strings.TrimSpace(getenv(EnvTrimBootDelay))); err == nil {
		delay = d
	}
	if delay <= 0 {
		return
	}
	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return
	case <-t.C:
	}
	debug.FreeOSMemory()
}
