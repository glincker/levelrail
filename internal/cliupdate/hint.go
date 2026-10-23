package cliupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	stateFile = "update-check.json"
	day       = 24 * time.Hour
	// retryAfter spaces attempts so an offline or slow network costs one try an hour, not one per command.
	retryAfter = time.Hour
)

type hintState struct {
	CheckedAt   time.Time `json:"checked_at"`
	AttemptedAt time.Time `json:"attempted_at"`
	Latest      string    `json:"latest"`
}

// Hinter prints a one-line notice at most once per Interval days.
type Hinter struct {
	Source  Source
	Current string
	// Dir is the CLI's config directory, where the check result is cached.
	Dir string
	// IntervalDays of 0 or less disables the hint.
	IntervalDays int
	Now          func() time.Time
}

// Check returns the line to print, or "" when there is nothing to say. It
// never returns an error: every failure just means no hint this time.
func (h Hinter) Check(ctx context.Context) string {
	if h.IntervalDays <= 0 || h.Current == "dev" || h.Dir == "" {
		return ""
	}
	now := time.Now
	if h.Now != nil {
		now = h.Now
	}
	path := filepath.Join(h.Dir, stateFile)
	st := readState(path)
	t := now()
	if !st.CheckedAt.IsZero() && t.Sub(st.CheckedAt) < time.Duration(h.IntervalDays)*day {
		return ""
	}
	if !st.AttemptedAt.IsZero() && t.Sub(st.AttemptedAt) < retryAfter {
		return ""
	}
	// Recorded before the fetch so a command that exits first still counts as an attempt.
	st.AttemptedAt = t
	writeState(path, st)
	latest, err := h.Source.Latest(ctx)
	if err != nil || latest == "" {
		return ""
	}
	st.CheckedAt, st.Latest = t, latest
	writeState(path, st)
	if !Newer(h.Current, latest) {
		return ""
	}
	return fmt.Sprintf("A newer release is available: %s (you have %s).", latest, h.Current)
}

func readState(path string) hintState {
	var st hintState
	data, err := os.ReadFile(path) //nolint:gosec // path is under the CLI's own config directory
	if err != nil {
		return st
	}
	if json.Unmarshal(data, &st) != nil {
		return hintState{}
	}
	return st
}

func writeState(path string, st hintState) {
	data, err := json.Marshal(st)
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(path), 0o700) != nil {
		return
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, data, 0o600) != nil {
		return
	}
	if os.Rename(tmp, path) != nil {
		_ = os.Remove(tmp)
	}
}
