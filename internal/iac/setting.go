package iac

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// liveSetting is one runtime setting as the control plane resolves it.
// allowWeaken is copied from the load context because the planner has none.
type liveSetting struct {
	runtimesettings.Setting
	allowWeaken bool
}

type allowWeakenKey struct{}

// WithAllowWeaken lets the Setting plan confirm a weakening change. Only the
// API sets it, after it has checked the caller is a human root session.
func WithAllowWeaken(ctx context.Context) context.Context {
	return context.WithValue(ctx, allowWeakenKey{}, true)
}

func allowWeakenFrom(ctx context.Context) bool {
	ok, _ := ctx.Value(allowWeakenKey{}).(bool)
	return ok
}

// SettingWeakenReason starts the reason of every Setting change that weakens
// a protection under allow_weaken, so reviewers and the audit can find it.
const SettingWeakenReason = "weakens a security protection; allowed by allow_weaken"

// IsSettingWeaken reports whether c is a Setting change that weakens a protection.
func IsSettingWeaken(c Change) bool {
	return c.Kind == KindSetting && c.Action == ActionUpdate && strings.HasPrefix(c.Reason, SettingWeakenReason)
}

func settingDefinition(key string) (runtimesettings.Definition, bool) {
	for _, d := range runtimesettings.Definitions() {
		if d.Key == key {
			return d, true
		}
	}
	return runtimesettings.Definition{}, false
}

// buildSetting accepts exactly one of value or reset: true. Type and range
// checks wait for the plan, which validates against the live definition.
func buildSetting(d *Document, s *issueSink) map[string]any {
	var sp struct {
		Value any  `yaml:"value"`
		Reset bool `yaml:"reset"`
	}
	decodeSpec(d, &sp, s)
	hasValue := mapValue(d.SpecNode, "value") != nil
	switch {
	case hasValue && sp.Reset:
		s.at("spec", "a Setting takes either value or reset: true, not both")
	case !hasValue && !sp.Reset:
		s.at("spec", "a Setting needs a value, or reset: true to go back to the default")
	case sp.Reset:
		return map[string]any{"reset": true}
	}
	return map[string]any{"value": sp.Value}
}

// settings reads each declared key once. A 404 is kept as an unknown key so
// the plan can name it instead of failing the whole read.
func (l loader) settings(ctx context.Context, res []*Resource) error {
	for _, r := range res {
		if r.Kind != KindSetting {
			continue
		}
		if _, seen := l.st.settings[r.Name]; seen {
			continue
		}
		var s liveSetting
		err := l.get(ctx, "/api/v1/settings/runtime/"+esc(r.Name), &s.Setting)
		switch {
		case err == nil:
			s.allowWeaken = allowWeakenFrom(ctx)
			l.st.settings[r.Name] = &s
		case IsNotFound(err):
			l.st.settings[r.Name] = nil
		default:
			if err := l.st.note(r.Key(), fmt.Errorf("read setting %s: %w", r.Name, err)); err != nil {
				return err
			}
		}
	}
	return nil
}

// mirrorStore holds only the live value, so a Service over it judges
// weakening against what the control plane runs now, not this process's env.
type mirrorStore struct{ rows []store.RuntimeSettingRow }

func (m mirrorStore) ListRuntimeSettings(context.Context) ([]store.RuntimeSettingRow, error) {
	return m.rows, nil
}

func (mirrorStore) SetRuntimeSetting(context.Context, string, string, string, time.Time) error {
	return errors.New("iac: the settings mirror is read only")
}

func (mirrorStore) DeleteRuntimeSetting(context.Context, string) error {
	return errors.New("iac: the settings mirror is read only")
}

func noEnv(string) (string, bool) { return "", false }

// settingMirror returns a validator whose current value is live's, and that
// value in canonical form ("" when the live value no longer parses).
func settingMirror(d runtimesettings.Definition, live *liveSetting) (*runtimesettings.Service, string) {
	canon, err := runtimesettings.ParseJSON(d, live.Value)
	if err != nil {
		return runtimesettings.NewWithDefinitions(mirrorStore{}, noEnv, []runtimesettings.Definition{d}), ""
	}
	rows := []store.RuntimeSettingRow{{Key: d.Key, Value: canon}}
	return runtimesettings.NewWithDefinitions(mirrorStore{rows: rows}, noEnv, []runtimesettings.Definition{d}), canon
}

const settingWeakenMsg = "setting %s: %s weakens a security protection; apply with allow_weaken from a human root session, or change it from the dashboard or the CLI"

// planSetting saves a declared value through the runtime settings PUT, with
// the same validator as the handler. It confirms a weakening change only
// when the API granted allow_weaken.
func (p *planner) planSetting(r *Resource) {
	if p.deniedItem(r, r.Key()) {
		return
	}
	live, known := p.st.settings[r.Name]
	def, defined := settingDefinition(r.Name)
	if !known || live == nil || !defined {
		p.fail(r, "unknown setting key %q; fix the name, or release or delete the document", r.Name)
		return
	}
	if def.Sensitive || live.Sensitive {
		p.fail(r, "setting %s holds a credential and cannot be declared in a file; set it from the dashboard or the CLI", r.Name)
		return
	}
	svc, cur := settingMirror(def, live)
	if def.Apply == runtimesettings.ApplyRestart {
		if live.PendingRestart {
			r.Warnings = append(r.Warnings, "saved, not effective until the control plane restarts")
		} else if !settingSettled(r, live, cur) {
			r.Warnings = append(r.Warnings, "takes effect after a control plane restart; apply never restarts it")
		}
	}
	if reset, _ := r.Fields["reset"].(bool); reset {
		p.planSettingReset(r, svc, def, live, cur)
		return
	}
	raw, err := json.Marshal(r.Fields["value"])
	if err != nil {
		p.fail(r, "setting %s: value cannot be encoded: %v", r.Name, err)
		return
	}
	weak := live.allowWeaken && svc.WeakensTo(context.Background(), r.Name, raw)
	next, err := svc.ValidateForWrite(context.Background(), r.Name, raw, runtimesettings.WriteOptions{RefuseSensitive: true, ConfirmWeakening: live.allowWeaken})
	if err != nil {
		p.failSettingWrite(r, err, "the new value")
		return
	}
	if next == cur {
		if live.Source == runtimesettings.SourceSaved {
			p.add(r, ActionNoop, nil, nil, nil)
			return
		}
		it := p.add(r, ActionUpdate, []FieldChange{{Path: "source", Op: OpChange, Old: live.Source, New: runtimesettings.SourceSaved}}, nil, settingPut(r.Name, raw, false))
		it.Reason = fmt.Sprintf("the value comes from the %s; save the current value so later env changes do not move it", live.Source)
		return
	}
	it := p.add(r, ActionUpdate, []FieldChange{{Path: "value", Op: OpChange, Old: cur, New: next}}, nil, settingPut(r.Name, raw, weak))
	if weak {
		it.Reason = SettingWeakenReason
	}
}

// planSettingReset deletes the saved row. Returning to a weaker default is
// refused unless the API granted allow_weaken.
func (p *planner) planSettingReset(r *Resource, svc *runtimesettings.Service, def runtimesettings.Definition, live *liveSetting, cur string) {
	if live.Source != runtimesettings.SourceSaved {
		p.add(r, ActionNoop, nil, nil, nil).Reason = "no saved value; the environment seed or the default already applies"
		return
	}
	weak := svc.WeakensTo(context.Background(), r.Name, live.Default)
	if weak && !live.allowWeaken {
		p.fail(r, settingWeakenMsg, r.Name, "the reset")
		return
	}
	dflt, err := runtimesettings.ParseJSON(def, live.Default)
	if err != nil {
		dflt = strings.TrimSpace(string(live.Default))
	}
	path := "/api/v1/settings/runtime/" + esc(r.Name)
	if weak {
		path += "?confirm_weakening=true"
	}
	it := p.add(r, ActionUpdate, []FieldChange{{Path: "value", Op: OpChange, Old: cur, New: dflt}}, nil, func(ctx context.Context, x *executor) error {
		return x.do(ctx, http.MethodDelete, path, nil, nil)
	})
	it.Reason = "reset: the saved value is removed, so the environment seed or the default applies"
	if weak {
		it.Reason = SettingWeakenReason + "; " + it.Reason
	}
}

// settingSettled reports whether the plan for r is a noop, judged before the
// full validation so the restart note only rides on a real change.
func settingSettled(r *Resource, live *liveSetting, cur string) bool {
	if reset, _ := r.Fields["reset"].(bool); reset {
		return live.Source != runtimesettings.SourceSaved
	}
	raw, err := json.Marshal(r.Fields["value"])
	if err != nil {
		return false
	}
	d, _ := settingDefinition(r.Name)
	next, err := runtimesettings.ParseJSON(d, raw)
	return err == nil && next == cur && live.Source == runtimesettings.SourceSaved
}

func (p *planner) failSettingWrite(r *Resource, err error, what string) {
	var verr *runtimesettings.ValidationError
	switch {
	case errors.Is(err, runtimesettings.ErrWeakeningNotConfirmed):
		p.fail(r, settingWeakenMsg, r.Name, what)
	case errors.Is(err, runtimesettings.ErrSensitiveKey):
		p.fail(r, "setting %s holds a credential and cannot be declared in a file; set it from the dashboard or the CLI", r.Name)
	case errors.Is(err, runtimesettings.ErrUnknownKey):
		p.fail(r, "unknown setting key %q; fix the name, or release or delete the document", r.Name)
	case errors.As(err, &verr):
		p.fail(r, "setting %s: %s", r.Name, verr.Message)
	default:
		p.fail(r, "setting %s: %v", r.Name, err)
	}
}

// settingPut sends confirm_weakening only for a granted weakening, so the
// server repeats the check for everything else.
func settingPut(key string, raw json.RawMessage, confirm bool) func(ctx context.Context, x *executor) error {
	body := map[string]any{"value": raw}
	if confirm {
		body["confirm_weakening"] = true
	}
	return func(ctx context.Context, x *executor) error {
		return x.do(ctx, http.MethodPut, "/api/v1/settings/runtime/"+esc(key), body, nil)
	}
}
