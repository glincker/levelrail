package objectstore

import (
	"fmt"
	"strings"
	"time"
)

const (
	serviceResourcePrefix = "service:"
	appSegment            = "service"
)

// ResourceID is the telemetry resource ID a log line of appName is stored under.
func ResourceID(appName string) string { return serviceResourcePrefix + appName }

func sanitizeSegment(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

func resourcePath(resourceID string) string {
	kind, name, ok := strings.Cut(resourceID, ":")
	if !ok {
		return sanitizeSegment(resourceID)
	}
	return sanitizeSegment(kind) + "/" + sanitizeSegment(name)
}

// AppPrefix is the key prefix holding one app's archived logs.
func AppPrefix(root, appName string) string {
	return root + "/" + appSegment + "/" + sanitizeSegment(appName) + "/"
}

// ObjectKey is root/<kind>/<name>/yyyy/mm/dd/hh/<start ns>.ndjson.gz. It is
// keyed by the chunk start only, so retrying a chunk overwrites it instead
// of leaving a duplicate; chunks are contiguous and sort by name.
func ObjectKey(root, resourceID string, startNs int64) string {
	t := time.Unix(0, startNs).UTC()
	return fmt.Sprintf("%s/%s/%s/%019d.ndjson.gz", root, resourcePath(resourceID), t.Format("2006/01/02/15"), startNs)
}

// appOfKey extracts the app name segment from a service log key, if any.
func appOfKey(root, key string) (string, bool) {
	rest, ok := strings.CutPrefix(key, root+"/"+appSegment+"/")
	if !ok {
		return "", false
	}
	app, _, ok := strings.Cut(rest, "/")
	return app, ok
}

// IsArchiveKey reports whether key is under the archive root and free of traversal.
func IsArchiveKey(root, key string) bool {
	return strings.HasPrefix(key, root+"/") && !strings.Contains(key, "..")
}

// hourWindows splits [startNs, endNs) at UTC hour boundaries.
func hourWindows(startNs, endNs int64, maxWindows int) [][2]int64 {
	hour := int64(time.Hour)
	var out [][2]int64
	for s := startNs; s < endNs && (maxWindows <= 0 || len(out) < maxWindows); {
		e := (s/hour + 1) * hour
		if e > endNs {
			e = endNs
		}
		out = append(out, [2]int64{s, e})
		s = e
	}
	return out
}
