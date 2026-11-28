package api

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/glincker/levelrail/internal/attention"
	"github.com/glincker/levelrail/internal/runmarker"
	"github.com/glincker/levelrail/internal/runtimesettings"
)

const (
	uncleanRestartDetail = "The control plane restarted after an unclean stop."
	uncleanOOMDetail     = "Likely out of memory: the host killed Levelrail. Add swap, lower build concurrency, set app memory limits, and check which containers use the most memory."
)

// uncleanRestartAttentionItems reports the newest unclean restart inside the
// retention window. It carries no process list or environment, only times.
func (rt *Router) uncleanRestartAttentionItems(ctx context.Context, abilities []string, now time.Time) []attention.Item {
	if rt.dataDir == "" || !hasAbility(abilities, AbilityRead) {
		return nil
	}
	window := rt.runtimeSettings.Duration(ctx, runtimesettings.KeyHostUncleanRestartWindow)
	if window <= 0 {
		window = runmarker.WindowFromEnv(os.Getenv)
	}
	recent := runmarker.Recent(rt.dataDir, window, now)
	if len(recent) == 0 {
		return nil
	}
	last := recent[0]
	detail := uncleanRestartDetail
	if last.OOMLikely {
		detail = uncleanOOMDetail
	}
	if len(recent) > 1 {
		detail = fmt.Sprintf("%s (%d times in the retention window)", detail, len(recent))
	}
	item := feedItem(attention.Warning, attention.KindUncleanRestart, "control-plane", detail, map[string]string{
		"at":         last.At.UTC().Format(time.RFC3339),
		"oom_likely": strconv.FormatBool(last.OOMLikely),
		"count":      strconv.Itoa(len(recent)),
	})
	return []attention.Item{item}
}
