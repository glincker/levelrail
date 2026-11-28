package appsleep

import (
	"context"
	"sort"
	"time"

	"github.com/glincker/levelrail/internal/store"
)

// appLister is the optional store call that finds the services of one app.
type appLister interface {
	ListServicesByApp(ctx context.Context, appID string) ([]store.DesiredService, error)
}

// stackMembers returns the services of name's multi-service app with
// dependencies first (depends_on order). A lone service returns just itself.
func stackMembers(ctx context.Context, apps Apps, name string) []string {
	l, ok := apps.(appLister)
	if !ok {
		return []string{name}
	}
	svc, err := apps.GetDesiredService(ctx, name)
	if err != nil || svc.AppID == "" {
		return []string{name}
	}
	list, err := l.ListServicesByApp(ctx, svc.AppID)
	if err != nil || len(list) < 2 {
		return []string{name}
	}
	return dependencyOrder(svc.AppID, list)
}

// dependencyOrder sorts so every service follows the ones it depends on; a
// cycle falls back to name order for the remainder rather than failing.
func dependencyOrder(appID string, list []store.DesiredService) []string {
	byName := make(map[string]store.DesiredService, len(list))
	names := make([]string, 0, len(list))
	for _, s := range list {
		byName[s.Name] = s
		names = append(names, s.Name)
	}
	sort.Strings(names)
	done := map[string]bool{}
	out := make([]string, 0, len(names))
	var visit func(n string, depth int)
	visit = func(n string, depth int) {
		if done[n] || depth > len(names) {
			return
		}
		done[n] = true
		s := byName[n]
		deps := append(append([]string{}, s.DependsOn...), s.DependsOnHealthy...)
		sort.Strings(deps)
		for _, d := range deps {
			if _, ok := byName[appID+"-"+d]; ok {
				visit(appID+"-"+d, depth+1)
			}
		}
		out = append(out, n)
	}
	for _, n := range names {
		visit(n, 0)
	}
	return out
}

// hasSleepRow is true when name has sleep switched on (the row ListAppSleep returns).
func hasSleepRow(ctx context.Context, sleep any, name string) (store.AppSleep, bool) {
	g, ok := sleep.(interface {
		GetAppSleep(ctx context.Context, serviceName string) (store.AppSleep, error)
	})
	if !ok {
		return store.AppSleep{}, false
	}
	a, err := g.GetAppSleep(ctx, name)
	return a, err == nil && a.IdleMinutes > 0
}

// stackStop stops every member of name's stack, dependents first, and
// marks the ones with a sleep row as sleeping.
func stackStop(ctx context.Context, sleep any, apps Apps, name string, now time.Time) error {
	members := stackMembers(ctx, apps, name)
	for i := len(members) - 1; i >= 0; i-- {
		m := members[i]
		if _, ok := hasSleepRow(ctx, sleep, m); ok && m != name {
			if s, ok := sleep.(interface {
				SetAppSleeping(ctx context.Context, serviceName string, sleeping bool, now time.Time) error
			}); ok {
				if err := s.SetAppSleeping(ctx, m, true, now); err != nil {
					return err
				}
			}
		}
		if err := apps.UpdateServiceSuspended(ctx, m, true); err != nil {
			return err
		}
	}
	return nil
}

// stackStart starts every member of name's stack in depends_on order so the
// entry service comes up after what it needs. Sleeping marks clear first.
func stackStart(ctx context.Context, sleep interface {
	SetAppSleeping(ctx context.Context, serviceName string, sleeping bool, now time.Time) error
}, apps Apps, name string, now time.Time) error {
	members := stackMembers(ctx, apps, name)
	for _, m := range members {
		if a, ok := hasSleepRow(ctx, sleep, m); ok && a.Sleeping && m != name {
			if err := sleep.SetAppSleeping(ctx, m, false, now); err != nil {
				return err
			}
		}
	}
	for _, m := range members {
		if err := apps.UpdateServiceSuspended(ctx, m, false); err != nil {
			return err
		}
	}
	return nil
}

// stackPlan maps each opted-in app to its stack members (only multi-service
// stacks, when stack sleep is on) and extends settings with stand-in rows for
// members that have no sleep setting, so their network use is read too.
func (s *Scheduler) stackPlan(ctx context.Context, settings []store.AppSleep, cfg Config) (map[string][]string, []store.AppSleep) {
	stacks := map[string][]string{}
	if !cfg.StackSleep {
		return stacks, settings
	}
	all := append([]store.AppSleep{}, settings...)
	have := map[string]bool{}
	for _, a := range settings {
		have[a.ServiceName] = true
	}
	for _, a := range settings {
		members := stackMembers(ctx, s.Apps, a.ServiceName)
		if len(members) < 2 {
			continue
		}
		stacks[a.ServiceName] = members
		for _, m := range members {
			if !have[m] {
				have[m] = true
				all = append(all, store.AppSleep{ServiceName: m, IdleMinutes: a.IdleMinutes, IgnorePaths: a.IgnorePaths})
			}
		}
	}
	return stacks, all
}

// stackSignals merges the evidence of every member: any recent request,
// network moment, connection or running operation on one keeps all awake. It
// reports false when a member cannot be read or any opted-in member is blocked.
func (s *Scheduler) stackSignals(ctx context.Context, members []string, rows map[string]store.AppSleep, lead store.AppSleep, latest, net map[string]time.Time, ops map[string]bool, now time.Time, fleet *Fleet) (Signals, time.Time, bool) {
	var sig Signals
	since := lead.Since
	for _, m := range members {
		row, opted := rows[m]
		if !opted {
			row = store.AppSleep{ServiceName: m, IdleMinutes: lead.IdleMinutes, IgnorePaths: lead.IgnorePaths}
		} else if row.Since.After(since) {
			since = row.Since
		}
		svc, err := s.Apps.GetDesiredService(ctx, m)
		if err != nil {
			return sig, since, false
		}
		if opted {
			facts := Facts{App: m, PinnedPorts: PinnedPorts(svc), PortWakeEnabled: row.PortWake}
			if fleet != nil {
				facts = fleet.Facts(svc, row, s.PortWake)
			}
			if len(Evaluate(facts).Blockers) > 0 {
				return sig, since, false
			}
		}
		ms := s.signals(ctx, svc, row, latest[resourceID(m)], now, net[m])
		if ms.LastRequest.After(sig.LastRequest) {
			sig.LastRequest = ms.LastRequest
		}
		if ms.LastNetwork.After(sig.LastNetwork) {
			sig.LastNetwork = ms.LastNetwork
		}
		sig.OpenConnections += ms.OpenConnections
		sig.OperationInFlight = sig.OperationInFlight || ops[m]
	}
	return sig, since, true
}
