package migrationplan

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Cutover checklist steps, in the only order they can be completed.
const (
	StepImported      = "imported"
	StepDataVerified  = "data_verified"
	StepShadowRunning = "shadow_running"
	StepDNSSwitched   = "dns_switched"
	StepOldStopped    = "old_stopped"
)

// Steps is the checklist order.
var Steps = []string{StepImported, StepDataVerified, StepShadowRunning, StepDNSSwitched, StepOldStopped}

// Step statuses.
const (
	StatusPending = "pending"
	StatusDone    = "done"
)

// Transition errors.
var (
	ErrUnknownStep  = errors.New("unknown checklist step")
	ErrStepBlocked  = errors.New("an earlier step is not done")
	ErrStepNotDone  = errors.New("step is not done")
	ErrAlreadyDone  = errors.New("step is already done")
	ErrBadChecklist = errors.New("stored checklist is not valid")
)

// StepState is one step's stored state.
type StepState struct {
	Status   string   `json:"status"`
	At       string   `json:"at,omitempty"`
	Actor    string   `json:"actor,omitempty"`
	Note     string   `json:"note,omitempty"`
	Evidence []string `json:"evidence,omitempty"`
}

// Checklist is the per-app state machine. Zero value has every step pending.
type Checklist struct {
	States map[string]StepState `json:"states"`
}

// New returns a checklist with every step pending.
func New() Checklist {
	c := Checklist{States: map[string]StepState{}}
	for _, s := range Steps {
		c.States[s] = StepState{Status: StatusPending}
	}
	return c
}

// Parse decodes stored JSON, filling any missing step as pending.
func Parse(raw string) (Checklist, error) {
	c := New()
	if strings.TrimSpace(raw) == "" || raw == "{}" {
		return c, nil
	}
	var in Checklist
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return c, fmt.Errorf("%w: %v", ErrBadChecklist, err)
	}
	for k, v := range in.States {
		if stepIndex(k) < 0 {
			continue
		}
		if v.Status != StatusDone {
			v.Status = StatusPending
		}
		c.States[k] = v
	}
	return c, nil
}

// Marshal encodes the checklist for storage.
func (c Checklist) Marshal() string {
	b, err := json.Marshal(c)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func stepIndex(step string) int {
	for i, s := range Steps {
		if s == step {
			return i
		}
	}
	return -1
}

// Current is the first step not done, or "" when the migration is finished.
func (c Checklist) Current() string {
	for _, s := range Steps {
		if c.States[s].Status != StatusDone {
			return s
		}
	}
	return ""
}

// Complete marks step done. Every earlier step must already be done.
func (c *Checklist) Complete(step, actor, note string, evidence []string, now time.Time) error {
	i := stepIndex(step)
	if i < 0 {
		return fmt.Errorf("%w: %q", ErrUnknownStep, step)
	}
	if c.States[step].Status == StatusDone {
		return fmt.Errorf("%w: %s", ErrAlreadyDone, step)
	}
	for _, prev := range Steps[:i] {
		if c.States[prev].Status != StatusDone {
			return fmt.Errorf("%w: finish %s first", ErrStepBlocked, prev)
		}
	}
	c.States[step] = StepState{Status: StatusDone, At: now.UTC().Format(time.RFC3339), Actor: actor, Note: note, Evidence: evidence}
	return nil
}

// Reset reopens step and every later step, since they depended on it.
func (c *Checklist) Reset(step string) error {
	i := stepIndex(step)
	if i < 0 {
		return fmt.Errorf("%w: %q", ErrUnknownStep, step)
	}
	if c.States[step].Status != StatusDone {
		return fmt.Errorf("%w: %s", ErrStepNotDone, step)
	}
	for _, s := range Steps[i:] {
		c.States[s] = StepState{Status: StatusPending}
	}
	return nil
}

// Progress counts done steps.
func (c Checklist) Progress() int {
	n := 0
	for _, s := range Steps {
		if c.States[s].Status == StatusDone {
			n++
		}
	}
	return n
}
