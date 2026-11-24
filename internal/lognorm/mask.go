package lognorm

import (
	"regexp"

	"github.com/GLINCKER/levelrail/internal/secretmask"
)

// Placeholders left in a template so equal failures with different values cluster.
const (
	PhTimestamp = "<TS>"
	PhUUID      = "<UUID>"
	PhIP        = "<IP>"
	PhHex       = "<HEX>"
	PhNum       = "<NUM>"
)

// Order matters: wider shapes first so a timestamp is not eaten as numbers.
var (
	tsRe   = regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:[.,]\d+)?(?:Z|[+-]\d{2}:?\d{2})?|\b[A-Z][a-z]{2} +\d{1,2} \d{2}:\d{2}:\d{2}\b|\b\d{2}:\d{2}:\d{2}(?:[.,]\d+)?\b`)
	uuidRe = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
	ipRe   = regexp.MustCompile(`\b\d{1,3}(?:\.\d{1,3}){3}(?::\d{1,5})?\b|\b(?:[0-9a-fA-F]{1,4}:){2,7}[0-9a-fA-F]{1,4}\b`)
	hexRe  = regexp.MustCompile(`\b0x[0-9a-fA-F]+\b|\b[0-9a-f]{8,}\b`)
	numRe  = regexp.MustCompile(`\b\d+(?:\.\d+)?`)
)

// Sample returns the line with known secrets masked. This is the only form
// that may be stored; reg nil uses the process-wide registry.
func Sample(line string, reg *secretmask.Registry) string {
	if reg == nil {
		return secretmask.Mask(line)
	}
	return reg.Mask(line)
}

// Template masks secrets first, then replaces variable parts with placeholders.
func Template(line string, reg *secretmask.Registry) string {
	return Placeholders(Sample(line, reg))
}

// Placeholders swaps timestamps, uuids, ips, hex ids and numbers for tokens.
func Placeholders(s string) string {
	s = tsRe.ReplaceAllString(s, PhTimestamp)
	s = uuidRe.ReplaceAllString(s, PhUUID)
	s = ipRe.ReplaceAllString(s, PhIP)
	s = hexRe.ReplaceAllString(s, PhHex)
	return numRe.ReplaceAllString(s, PhNum)
}

// Entry is one normalized record ready for a miner.
type Entry struct {
	Sample   string
	Template string
	Level    string
}

// Normalize runs join, field pick and mask over raw lines in order.
func Normalize(lines []string, reg *secretmask.Registry) []Entry {
	joined := Join(lines)
	out := make([]Entry, 0, len(joined))
	for _, l := range joined {
		rec := Extract(l)
		text := rec.Text()
		if text == "" {
			continue
		}
		out = append(out, Entry{Sample: Sample(text, reg), Template: Template(text, reg), Level: rec.Level})
	}
	return out
}
