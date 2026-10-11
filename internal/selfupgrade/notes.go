package selfupgrade

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/kit/semver"
)

// MetaMarker opens the machine-readable upgrade metadata comment a release
// body may carry. The JSON inside lists breaking changes explicitly.
const MetaMarker = "levelrail-upgrade:"

const (
	maxBreakingPerRelease = 20
	maxSummaryRunes       = 400
)

// Breaking is one breaking change between two versions. RequiresAck means the
// upgrade refuses to start until the operator acknowledges it by id.
type Breaking struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	Summary     string `json:"summary"`
	RequiresAck bool   `json:"requires_ack"`
	Source      string `json:"source"`
}

// Breaking change sources.
const (
	SourceMetadata = "metadata"
	SourceHeading  = "heading"
)

// ReleaseNotes is one release's tag and notes body.
type ReleaseNotes struct {
	Tag  string
	Body string
}

type metaBlock struct {
	Breaking []struct {
		ID      string `json:"id"`
		Summary string `json:"summary"`
		Ack     *bool  `json:"ack"`
	} `json:"breaking"`
}

var (
	metaPattern    = regexp.MustCompile(`(?s)<!--\s*` + regexp.QuoteMeta(MetaMarker) + `\s*(\{.*?\})\s*-->`)
	headingPattern = regexp.MustCompile(`^#{2,4}\s*(?:\S{1,4}\s+)?breaking(?:\s+changes?)?\s*$`)
	idCleaner      = regexp.MustCompile(`[^a-z0-9-]+`)
)

// ParseBreaking extracts the breaking changes one release body declares. The
// explicit metadata comment wins; without it, bullets under a "Breaking
// changes" heading count and always require acknowledgement.
func ParseBreaking(tag, body string) []Breaking {
	if m := metaPattern.FindStringSubmatch(body); m != nil {
		var meta metaBlock
		if err := json.Unmarshal([]byte(m[1]), &meta); err == nil && len(meta.Breaking) > 0 {
			var out []Breaking
			for _, b := range meta.Breaking {
				summary := trimSummary(b.Summary)
				if summary == "" {
					continue
				}
				id := idCleaner.ReplaceAllString(strings.ToLower(strings.TrimSpace(b.ID)), "-")
				if id == "" || id == "-" {
					id = stableID(tag, summary)
				}
				ack := b.Ack == nil || *b.Ack
				out = append(out, Breaking{ID: id, Version: tag, Summary: summary, RequiresAck: ack, Source: SourceMetadata})
				if len(out) == maxBreakingPerRelease {
					break
				}
			}
			return out
		}
	}
	return headingBreaking(tag, body)
}

func headingBreaking(tag, body string) []Breaking {
	var out []Breaking
	in := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "#"):
			in = headingPattern.MatchString(strings.ToLower(trimmed))
		case in && (strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "- ")):
			summary := trimSummary(strings.TrimSpace(trimmed[2:]))
			if summary == "" {
				continue
			}
			out = append(out, Breaking{ID: stableID(tag, summary), Version: tag, Summary: summary, RequiresAck: true, Source: SourceHeading})
			if len(out) == maxBreakingPerRelease {
				return out
			}
		}
	}
	return out
}

func stableID(tag, summary string) string {
	sum := sha256.Sum256([]byte(tag + "\x00" + summary))
	return "b-" + hex.EncodeToString(sum[:4])
}

func trimSummary(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > maxSummaryRunes {
		return string(r[:maxSummaryRunes]) + "..."
	}
	return s
}

// BreakingBetween collects the breaking changes of every release newer than
// current and no newer than target, oldest first. Versions that do not order
// are skipped: an unknown tag cannot be said to sit between the two.
func BreakingBetween(releases []ReleaseNotes, current, target string) []Breaking {
	type item struct {
		tag string
		b   []Breaking
	}
	var picked []item
	for _, r := range releases {
		newer, ok1 := semver.Compare(r.Tag, current)
		upTo, ok2 := semver.Compare(r.Tag, target)
		if !ok1 || !ok2 || newer <= 0 || upTo > 0 {
			continue
		}
		if b := ParseBreaking(r.Tag, r.Body); len(b) > 0 {
			picked = append(picked, item{tag: r.Tag, b: b})
		}
	}
	sort.SliceStable(picked, func(i, j int) bool {
		c, ok := semver.Compare(picked[i].tag, picked[j].tag)
		return ok && c < 0
	})
	var out []Breaking
	for _, p := range picked {
		out = append(out, p.b...)
	}
	return out
}

// MissingAcks returns the breaking changes that require acknowledgement and
// were not acknowledged.
func MissingAcks(breaking []Breaking, acked []string) []Breaking {
	have := make(map[string]bool, len(acked))
	for _, id := range acked {
		have[id] = true
	}
	var out []Breaking
	for _, b := range breaking {
		if b.RequiresAck && !have[b.ID] {
			out = append(out, b)
		}
	}
	return out
}

// AckError names the unacknowledged breaking changes in a refusal message.
func AckError(missing []Breaking) error {
	ids := make([]string, 0, len(missing))
	for _, m := range missing {
		ids = append(ids, m.ID)
	}
	return fmt.Errorf("%w: acknowledge %s (pass --ack %s)", ErrBreakingNotAcked, strings.Join(ids, ", "), strings.Join(ids, ","))
}
