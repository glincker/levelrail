package iac

import (
	"context"
	"net/http"
)

func (st *State) channelName(id string) string {
	for name, cid := range st.channels {
		if cid == id {
			return name
		}
	}
	return ""
}

func alertFromWire(st *State, a wireAlert) map[string]any {
	f := map[string]any{"type": a.Kind, "enabled": a.Enabled}
	putIf(f, "metric", a.Metric)
	putIf(f, "comparator", a.Comparator)
	if d, err := normDuration(a.ForDuration); err == nil {
		putIf(f, "for", d)
	}
	if d, err := normDuration(a.RestartWindow); err == nil {
		putIf(f, "restartWindow", d)
	}
	putIf(f, "channel", st.channelName(a.ChannelID))
	if a.Threshold != 0 {
		f["threshold"] = a.Threshold
	}
	if a.RestartCountThreshold > 0 {
		f["restartCountThreshold"] = a.RestartCountThreshold
	}
	return f
}

func alertBody(r *Resource, channelID string) map[string]any {
	f := r.Fields
	body := map[string]any{"name": r.Name, "kind": f["type"], "enabled": f["enabled"]}
	for k, wire := range map[string]string{"metric": "metric", "comparator": "comparator", "threshold": "threshold", "for": "for_duration", "restartCountThreshold": "restart_count_threshold", "restartWindow": "restart_window"} {
		if v, ok := f[k]; ok {
			body[wire] = v
		}
	}
	if channelID != "" {
		body["channel_id"] = channelID
	}
	return body
}

func (p *planner) planAlert(r *Resource) {
	if p.deniedItem(r, "alerts/"+r.Scope) || p.deniedItem(r, "channels") {
		return
	}
	if !p.appKnown(r.Scope) {
		p.fail(r, "app %q does not exist and is not declared in the files", r.Scope)
		return
	}
	var channelID string
	if ch := stringField(r.Fields, "channel"); ch != "" {
		id, ok := p.st.channels[ch]
		if !ok {
			p.fail(r, "notification channel %q does not exist", ch)
			return
		}
		channelID = id
	}
	base := "/api/v1/apps/" + esc(r.Scope) + "/alerts"
	live, ok := p.st.alerts[r.Scope][r.Name]
	if !ok {
		p.add(r, ActionCreate, addAll(r.Fields, nil), nil, func(ctx context.Context, x *executor) error {
			return x.do(ctx, http.MethodPost, base, alertBody(r, channelID), nil)
		})
		return
	}
	changes, _ := diffFields(r.Fields, alertFromWire(p.st, live), nil, false)
	if len(changes) == 0 {
		p.add(r, ActionNoop, nil, nil, nil)
		return
	}
	p.add(r, ActionUpdate, changes, nil, func(ctx context.Context, x *executor) error {
		return x.do(ctx, http.MethodPut, base+"/"+esc(live.ID), alertBody(r, channelID), nil)
	})
}
