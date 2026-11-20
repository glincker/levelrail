// Package pages holds Pages logic that does not belong to a single HTTP route.
package pages

import (
	"fmt"
	"strings"
)

// Event names accepted by the notify event filter.
const (
	EventSuccess = "success"
	EventFailure = "failure"
)

// NotifyConfig is one site's deploy notification switch and event filter.
type NotifyConfig struct {
	Enabled   bool `json:"enabled"`
	OnSuccess bool `json:"on_success"`
	OnFailure bool `json:"on_failure"`
}

// DefaultNotifyConfig is what a site without a stored row behaves like.
func DefaultNotifyConfig() NotifyConfig {
	return NotifyConfig{Enabled: true, OnSuccess: true, OnFailure: true}
}

// Allows reports whether a deploy outcome should be sent.
func (c NotifyConfig) Allows(succeeded bool) bool {
	if !c.Enabled {
		return false
	}
	if succeeded {
		return c.OnSuccess
	}
	return c.OnFailure
}

// Events lists the enabled event names in a stable order.
func (c NotifyConfig) Events() []string {
	out := []string{}
	if c.OnSuccess {
		out = append(out, EventSuccess)
	}
	if c.OnFailure {
		out = append(out, EventFailure)
	}
	return out
}

// ParseEvents turns event names into filter flags; at least one is required.
func ParseEvents(events []string) (onSuccess, onFailure bool, err error) {
	for _, e := range events {
		switch strings.ToLower(strings.TrimSpace(e)) {
		case EventSuccess:
			onSuccess = true
		case EventFailure:
			onFailure = true
		case "":
		default:
			return false, false, fmt.Errorf("unknown event %q, use %s or %s", e, EventSuccess, EventFailure)
		}
	}
	if !onSuccess && !onFailure {
		return false, false, fmt.Errorf("at least one event is required: %s, %s", EventSuccess, EventFailure)
	}
	return onSuccess, onFailure, nil
}
