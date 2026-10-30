package templateupgrade

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/compose"
)

// HealthcheckChange summarizes a probe change; Preserved means the user
// already edited it away from the baseline.
type HealthcheckChange struct {
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
	Preserved bool   `json:"preserved,omitempty"`
}

func healthSummary(s compose.Service) string {
	var parts []string
	if hc := s.Healthcheck; hc != nil {
		parts = append(parts, strings.Join([]string(hc.Test), " "), hc.Interval, hc.Timeout, hc.StartPeriod)
		if hc.Retries > 0 {
			parts = append(parts, strconv.Itoa(hc.Retries))
		}
	}
	if s.PlatformProbe != nil {
		if b, err := json.Marshal(s.PlatformProbe); err == nil {
			parts = append(parts, string(b))
		}
	}
	return strings.TrimSpace(strings.Join(parts, "|"))
}

func planHealthcheck(inst, tpl compose.Service, base *compose.Service, sig *signals) *HealthcheckChange {
	from, to := healthSummary(inst), healthSummary(tpl)
	if from == to {
		return nil
	}
	ch := &HealthcheckChange{From: from, To: to}
	if base != nil && healthSummary(*base) != from {
		ch.Preserved = true
		return ch
	}
	sig.otherChange = true
	return ch
}
