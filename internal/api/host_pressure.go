package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/hostpressure"
	"github.com/GLINCKER/levelrail/internal/hostswap"
)

const (
	eventHostPressure = "host.pressure"
	docsHostPressure  = "/host-pressure"
)

// SetHostPressure enables the host pressure doctor rows, attention item and event.
func (rt *Router) SetHostPressure(g *hostpressure.Guard) {
	rt.hostPressure = g
	g.OnChange = func(st hostpressure.Status) {
		tops := make([]map[string]any, 0, len(st.TopConsumers))
		for _, c := range st.TopConsumers {
			tops = append(tops, map[string]any{"name": c.Name, "mem_bytes": c.MemBytes})
		}
		rt.EmitEvent(eventHostPressure, map[string]any{
			"state": string(st.State), "reasons": st.Reasons, "top_consumers": tops, "actions": st.Actions,
		})
	}
}

func topConsumerText(st hostpressure.Status) string {
	if len(st.TopConsumers) == 0 {
		return ""
	}
	parts := make([]string, 0, len(st.TopConsumers))
	for _, c := range st.TopConsumers {
		parts = append(parts, fmt.Sprintf("%s %s", c.Name, humanBytes(c.MemBytes)))
	}
	return "Top memory users: " + strings.Join(parts, ", ") + ". "
}

func (rt *Router) doctorCheckHostPressure(context.Context) doctorCheckResource {
	const code, name = "host_pressure", "Host pressure"
	if rt.hostPressure == nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "Host pressure sampling is not running."}
	}
	st := rt.hostPressure.Status()
	if st.Source == "unavailable" {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "This host does not report memory or pressure data."}
	}
	switch st.State {
	case hostpressure.StateCritical, hostpressure.StateWarn:
		status, word := doctorStatusWarn, "warm"
		if st.State == hostpressure.StateCritical {
			status, word = doctorStatusFail, "critical"
		}
		return doctorCheckResource{
			Code: code, Name: name, Status: status,
			Message:  fmt.Sprintf("Host pressure is %s (%s). %sNew builds %s.", word, strings.Join(st.Reasons, ", "), topConsumerText(st), shedPhrase(st.State)),
			Fix:      strings.Join(st.Actions, "; "),
			DocsPath: docsHostPressure,
		}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: fmt.Sprintf("CPU and memory pressure are normal (read from %s).", st.Source)}
}

func shedPhrase(s hostpressure.State) string {
	if s == hostpressure.StateCritical {
		return "wait until it recovers"
	}
	return "run one at a time"
}

func (rt *Router) doctorCheckNoSwap(context.Context) doctorCheckResource {
	const code, name = "host_swap", "Swap"
	if rt.hostPressure == nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "Host pressure sampling is not running."}
	}
	st := rt.hostPressure.Status()
	host := hostswap.Read("")
	detail := ""
	if host.Supported {
		detail = " (" + host.Summary() + ")"
	}
	if st.NoSwap {
		return doctorCheckResource{
			Code: code, Name: name, Status: doctorStatusWarn,
			Message:  fmt.Sprintf("No swap is configured and the host has %s of RAM. A memory spike can make the kernel kill processes, including the control plane.%s", humanBytes(st.Sample.MemTotal), detail),
			Fix:      "On the control plane host run the CLI command 'swap' to print the exact commands (dry run), then 'sudo <cli> swap --apply' to create the swapfile. Nothing changes until you apply it.",
			DocsPath: docsHostPressure,
		}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "Swap is configured, or the host has enough RAM to run without it." + detail}
}

// hostPressureAttentionItems raises one item while the host is warm or critical.
func (rt *Router) hostPressureAttentionItems(abilities []string) []attention.Item {
	if rt.hostPressure == nil || !hasAbility(abilities, AbilityRoot) {
		return nil
	}
	st := rt.hostPressure.Status()
	if st.State == hostpressure.StateOK {
		return nil
	}
	sev := attention.Warning
	if st.State == hostpressure.StateCritical {
		sev = attention.Critical
	}
	detail := fmt.Sprintf("%s. %s%s", strings.Join(st.Reasons, ", "), topConsumerText(st), strings.Join(st.Actions, "; "))
	return []attention.Item{feedItem(sev, attention.KindHostPressure, "host", detail, map[string]string{"state": string(st.State)})}
}
