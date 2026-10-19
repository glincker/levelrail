package plugin

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/GLINCKER/levelrail/internal/plugin/pluginpb"
)

// Panel call failures. Callers match with errors.Is.
var (
	ErrNoPanel  = errors.New("plugin: no such panel, source or action")
	ErrNotPanel = errors.New("plugin: not a panel provider")

	// ErrCallFailed wraps a transport or plugin error from a panel call.
	ErrCallFailed = errors.New("plugin: call failed")
)

// ScopeKeyAppID is the only scope key a panel call may carry in this release.
const ScopeKeyAppID = "app_id"

var scopeValuePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

// ActionResult is the plugin's answer to a button press.
type ActionResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// PanelData asks the plugin for the panel's data and binds it to the declared
// widgets. Only the panel's declared source abilities that the operator
// approved are used, and the call is bounded by the call timeout and the
// panel response cap.
func (m *Manager) PanelData(ctx context.Context, name, panelID string, scope map[string]string) (PanelView, error) {
	rec, panel, err := m.panelTarget(ctx, name, panelID)
	if err != nil {
		return PanelView{}, err
	}
	scope, err = cleanScope(scope)
	if err != nil {
		return PanelView{}, err
	}
	in := m.inst(name)
	client, err := m.admit(in)
	if err != nil {
		return PanelView{}, err
	}
	defer func() { <-in.sem }()
	if err := m.auditPanelUse(ctx, name, "panel_data", panel, panelID); err != nil {
		return PanelView{}, err
	}
	cctx, cancel := context.WithTimeout(ctx, m.cfg.CallTimeout)
	defer cancel()
	resp, err := client.PanelData(cctx, panelDataRequest(panel, panelID, scope, rec.ConfigJSON))
	if err != nil {
		m.noteCallResult(in, false)
		return PanelView{}, callError(cctx, "panel data", name, err)
	}
	m.noteCallResult(in, true)
	if len(resp.GetJson()) > m.cfg.MaxPanelBytes {
		return PanelView{}, ErrTooLarge
	}
	return BindPanel(panel, []byte(resp.GetJson()))
}

// PanelAction presses a declared button. The action id must belong to a
// button of the panel, so a caller can never reach an arbitrary plugin method.
func (m *Manager) PanelAction(ctx context.Context, name, panelID, actionID string, scope map[string]string) (ActionResult, error) {
	rec, panel, err := m.panelTarget(ctx, name, panelID)
	if err != nil {
		return ActionResult{}, err
	}
	if !panel.hasAction(actionID) {
		return ActionResult{}, ErrNoPanel
	}
	scope, err = cleanScope(scope)
	if err != nil {
		return ActionResult{}, err
	}
	in := m.inst(name)
	client, err := m.admit(in)
	if err != nil {
		return ActionResult{}, err
	}
	defer func() { <-in.sem }()
	if err := m.auditPanelUse(ctx, name, "panel_action:"+actionID, panel, panelID); err != nil {
		return ActionResult{}, err
	}
	cctx, cancel := context.WithTimeout(ctx, m.cfg.CallTimeout)
	defer cancel()
	resp, err := client.PanelAction(cctx, panelActionRequest(panelID, actionID, scope, rec.ConfigJSON))
	if err != nil {
		m.noteCallResult(in, false)
		return ActionResult{}, callError(cctx, "panel action", name, err)
	}
	m.noteCallResult(in, true)
	return ActionResult{OK: resp.GetOk(), Message: SanitizeMarkdown(truncateUTF8(resp.GetMessage(), m.cfg.MaxResponseBytes))}, nil
}

// panelTarget resolves an enabled panel provider and a panel with a source,
// and refuses unless the operator approved every ability the source uses.
func (m *Manager) panelTarget(ctx context.Context, name, panelID string) (Record, Panel, error) {
	rec, err := m.store.GetPlugin(ctx, name)
	if err != nil {
		return Record{}, Panel{}, err
	}
	if rec.Kind != KindPanelProvider {
		return Record{}, Panel{}, ErrNotPanel
	}
	if rec.State != StateEnabled {
		return Record{}, Panel{}, fmt.Errorf("%w: plugin is %s", ErrUnavailable, rec.State)
	}
	man, err := ParseManifest([]byte(rec.Manifest))
	if err != nil {
		return Record{}, Panel{}, err
	}
	panel, ok := man.Panel(panelID)
	if !ok || panel.Source == nil {
		return Record{}, Panel{}, ErrNoPanel
	}
	for _, a := range panel.Source.Abilities {
		if !slices.Contains(rec.ApprovedAbilities, a) {
			return Record{}, Panel{}, ErrNotApproved
		}
	}
	return rec, panel, nil
}

func (m *Manager) auditPanelUse(ctx context.Context, name, call string, p Panel, panelID string) error {
	err := m.audit.Record(ctx, AuditEvent{Action: AuditAbilityUsed, Actor: "plugin:" + name, Plugin: name, Detail: map[string]string{
		"call": call, "panel": panelID, "abilities": strings.Join(p.Source.Abilities, ","),
	}})
	if err != nil {
		return fmt.Errorf("plugin: audit failed, call refused: %w", err)
	}
	return nil
}

func cleanScope(in map[string]string) (map[string]string, error) {
	out := map[string]string{}
	for k, v := range in {
		if k != ScopeKeyAppID || !scopeValuePattern.MatchString(v) {
			return nil, invalid("scope %q is not allowed", k)
		}
		out[k] = v
	}
	return out, nil
}

func panelDataRequest(p Panel, panelID string, scope map[string]string, cfg string) *pluginpb.PanelDataRequest {
	return &pluginpb.PanelDataRequest{PanelId: panelID, SourceId: p.Source.ID, Context: scope, ConfigJson: cfg}
}

func panelActionRequest(panelID, actionID string, scope map[string]string, cfg string) *pluginpb.PanelActionRequest {
	return &pluginpb.PanelActionRequest{PanelId: panelID, ActionId: actionID, Context: scope, ConfigJson: cfg}
}

// callError reports a timeout as context.DeadlineExceeded, which a gRPC status error would hide.
func callError(cctx context.Context, op, name string, err error) error {
	if cerr := cctx.Err(); cerr != nil {
		err = cerr
	}
	return fmt.Errorf("%w: %s on %q: %w", ErrCallFailed, op, name, err)
}
