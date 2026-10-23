package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/fixit"
	"github.com/GLINCKER/levelrail/internal/hostfw"
	"github.com/GLINCKER/levelrail/internal/store"
)

const auditActionWebPortsOpened = "host_firewall.web_ports_opened"

func commandsText(cmds []hostfw.Command) []string {
	out := make([]string, len(cmds))
	for i, c := range cmds {
		out[i] = c.String()
	}
	return out
}

// openWebPortsFix opens exactly ports 80 and 443 in the host firewall, and only
// when it is active and blocking them. It runs only as root outside a
// container, shows its commands and undo first, and is audited.
func (rt *Router) openWebPortsFix() fixit.Fix {
	return fixit.Fix{
		ID: openWebPortsFixID, CheckID: "firewall", Subject: "host", Kind: fixit.KindAuto,
		Title: "Open ports 80 and 443 in the host firewall",
		Why:   "Certificates and visitors reach this server on ports 80 and 443. The host firewall is blocking them.",
		Risk:  fixit.RiskExposes, Reversible: true, Needs: "root",
		Detect: func(ctx context.Context) (fixit.Applicability, error) {
			st := hostfw.Detect(ctx, rt.hostFirewallDeps())
			pl := hostfw.PlanOpen(st)
			a := fixit.Applicability{Failing: st.Blocking(), Detail: st.Detail, Changes: []fixit.Change{}}
			if !a.Failing {
				return a, nil
			}
			cmds := commandsText(pl.Add)
			for _, c := range cmds {
				a.Changes = append(a.Changes, fixit.Change{Label: "Run", Detail: c})
			}
			for _, c := range commandsText(pl.Undo) {
				a.Changes = append(a.Changes, fixit.Change{Label: "Undo with", Detail: c})
			}
			a.Command = strings.Join(cmds, " && ")
			if pl.Note != "" {
				a.Reason = pl.Note
			}
			if kind, why := rt.hostFirewallFixKind(); kind != fixit.KindAuto {
				a.Kind, a.Reason = kind, why
			}
			if pl.Refusal != "" {
				a.Kind, a.Reason = fixit.KindGuided, pl.Refusal
			}
			return a, nil
		},
		Run: func(ctx context.Context, in fixit.Input) error {
			if kind, why := rt.hostFirewallFixKind(); kind != fixit.KindAuto {
				return fmt.Errorf("%w: %s", fixit.ErrCannotFixHere, why)
			}
			res, err := hostfw.Apply(ctx, rt.hostFirewallDeps())
			if err != nil {
				rt.logger.Warn("api: open web ports failed", slog.String("error", err.Error()), slog.String("actor", in.Actor))
				if errors.Is(err, hostfw.ErrRefused) {
					return fmt.Errorf("%w: %w", fixit.ErrCannotFixHere, err)
				}
				return fmt.Errorf("open web ports: %w", err)
			}
			rt.recordFirewallAudit(ctx, in.Actor, res)
			rt.logger.Info("api: host firewall web ports opened", slog.String("backend", string(res.Plan.Backend)), slog.String("actor", in.Actor),
				slog.String("commands", strings.Join(commandsText(res.Plan.Add), "; ")), slog.String("undo", strings.Join(commandsText(res.Undo), "; ")))
			return nil
		},
	}
}

// recordFirewallAudit writes one audit row naming what ran and how to undo it.
func (rt *Router) recordFirewallAudit(ctx context.Context, actor string, res hostfw.Result) {
	if rt.auditLog == nil || len(res.Plan.Add) == 0 {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		return
	}
	entry := store.AuditEntry{
		ID: id, ActorType: "session", ActorName: actor, Ability: AbilityRoot, Method: http.MethodPost, StatusCode: http.StatusOK,
		Path:       "/api/v1/fixes/" + openWebPortsFixID + "/apply#" + strings.Join(commandsText(res.Plan.Add), "; "),
		CreatedAt:  store.FormatAuditTime(time.Now()),
		ClientKind: auditClientKindSystem, Action: auditActionWebPortsOpened,
	}
	if err := rt.auditLog.SaveAuditEntry(ctx, entry); err != nil {
		rt.logger.Warn("api: save firewall audit entry failed", slog.String("error", err.Error()))
	}
}
