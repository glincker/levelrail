// Package advisoryfeed reads a file based advisory feed and matches the
// images apps run against it. It is off unless a feed is configured.
package advisoryfeed

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// SchemaVersion is the only feed version this build understands.
const SchemaVersion = 1

// Severities, highest first.
const (
	SeverityCritical = "critical"
	SeverityHigh     = "high"
	SeverityMedium   = "medium"
	SeverityLow      = "low"
)

// Feed is the document an operator or a publisher provides.
type Feed struct {
	Version     int       `json:"version"`
	GeneratedAt time.Time `json:"generated_at,omitempty"`
	Entries     []Entry   `json:"entries"`
}

// Entry is one advisory.
type Entry struct {
	ID        string     `json:"id"`
	Severity  string     `json:"severity"`
	Summary   string     `json:"summary"`
	Affected  []Affected `json:"affected"`
	FixedIn   string     `json:"fixed_in,omitempty"`
	Reference string     `json:"reference,omitempty"`
}

// Affected names an image repository and the content that is affected. At
// least one of Digests, Tags or Ranges is required so an entry never matches
// a whole repository by accident.
type Affected struct {
	Repository string   `json:"repository"`
	Digests    []string `json:"digests,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Ranges     []Range  `json:"ranges,omitempty"`
}

// Range is a tag version interval, From inclusive and Before exclusive.
type Range struct {
	From   string `json:"from,omitempty"`
	Before string `json:"before"`
}

// Parse decodes and validates a feed document.
func Parse(data []byte) (Feed, error) {
	var f Feed
	if err := json.Unmarshal(data, &f); err != nil {
		return Feed{}, fmt.Errorf("advisory feed is not valid JSON: %w", err)
	}
	if f.Version != SchemaVersion {
		return Feed{}, fmt.Errorf("advisory feed version %d is not supported (want %d)", f.Version, SchemaVersion)
	}
	seen := map[string]bool{}
	for i, e := range f.Entries {
		if err := validateEntry(e); err != nil {
			return Feed{}, fmt.Errorf("advisory feed entry %d: %w", i, err)
		}
		if seen[e.ID] {
			return Feed{}, fmt.Errorf("advisory feed entry %d: duplicate id %q", i, e.ID)
		}
		seen[e.ID] = true
	}
	return f, nil
}

func validateEntry(e Entry) error {
	if strings.TrimSpace(e.ID) == "" {
		return errors.New("id is required")
	}
	switch e.Severity {
	case SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow:
	default:
		return fmt.Errorf("severity %q is not one of critical, high, medium, low", e.Severity)
	}
	if strings.TrimSpace(e.Summary) == "" {
		return errors.New("summary is required")
	}
	if len(e.Affected) == 0 {
		return errors.New("affected is required")
	}
	for _, a := range e.Affected {
		if NormalizeRepository(a.Repository) == "" {
			return errors.New("affected repository is required")
		}
		if len(a.Digests)+len(a.Tags)+len(a.Ranges) == 0 {
			return fmt.Errorf("affected %q needs digests, tags or ranges", a.Repository)
		}
		for _, r := range a.Ranges {
			if _, ok := parseVersion(r.Before); !ok {
				return fmt.Errorf("range before %q is not a dotted version", r.Before)
			}
			if r.From != "" {
				if _, ok := parseVersion(r.From); !ok {
					return fmt.Errorf("range from %q is not a dotted version", r.From)
				}
			}
		}
	}
	return nil
}

// SeverityRank orders severities, higher is worse; unknown is 0.
func SeverityRank(s string) int {
	switch s {
	case SeverityCritical:
		return 4
	case SeverityHigh:
		return 3
	case SeverityMedium:
		return 2
	case SeverityLow:
		return 1
	}
	return 0
}
