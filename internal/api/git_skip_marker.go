package api

import (
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/ghactions"
)

// skipMarkerIn returns the deploy-skip marker found in a commit message, matched case-insensitively.
func skipMarkerIn(message string) string {
	lower := strings.ToLower(message)
	for _, m := range ghactions.SkipMarkers {
		if strings.Contains(lower, m) {
			return m
		}
	}
	return ""
}

// skipPushForMarker returns a non-empty message when the pushed head commit opts out of deploying.
func skipPushForMarker(message string) string {
	if m := skipMarkerIn(message); m != "" {
		return fmt.Sprintf("ignored: head commit contains %s\n", m)
	}
	return ""
}
