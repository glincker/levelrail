package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	appEventActorSystem   = "system"
	appEventTitleKeyLimit = 5
)

// appEventStore is the optional store surface behind the app timeline and
// pending changes. *store.DB satisfies it; a Router over a store without it
// records nothing and reports no pending changes.
type appEventStore interface {
	AddAppEvent(ctx context.Context, e store.AppEvent) error
	ListAppEvents(ctx context.Context, name string, before *store.AppEventCursor, after time.Time, kinds []string, limit int) ([]store.AppEvent, error)
	GetAppliedConfig(ctx context.Context, name string) (*store.AppliedConfig, error)
}

func (rt *Router) appEvents() appEventStore {
	s, _ := rt.apps.(appEventStore)
	return s
}

// eventActor names who made the request: a user's display name, a token as
// "token:<name>", or "system" when the caller cannot be resolved.
func (rt *Router) eventActor(r *http.Request) string {
	actorType, _, name, ok := rt.currentActor(r)
	switch {
	case !ok || name == "":
		return appEventActorSystem
	case actorType == store.PrincipalTypeToken:
		return "token:" + name
	default:
		return name
	}
}

// recordAppEvent stores e for the request's app. Failures are logged, never
// returned: the change it describes already landed.
func (rt *Router) recordAppEvent(r *http.Request, e store.AppEvent) {
	s := rt.appEvents()
	if s == nil {
		return
	}
	e.Actor = rt.eventActor(r)
	if err := s.AddAppEvent(r.Context(), e); err != nil {
		rt.logger.Error("api: record app event failed", slog.String("error", err.Error()), slog.String("name", e.AppName), slog.String("kind", e.Kind))
	}
}

// keysTitle renders "prefix: A, B and 2 more" for a key-name list.
func keysTitle(prefix string, keys []string) string {
	if len(keys) <= appEventTitleKeyLimit {
		return prefix + ": " + strings.Join(keys, ", ")
	}
	return fmt.Sprintf("%s: %s and %d more", prefix, strings.Join(keys[:appEventTitleKeyLimit], ", "), len(keys)-appEventTitleKeyLimit)
}

// envKeyDiff returns the sorted key names added, changed and removed from
// old to next. Values are compared but never returned.
func envKeyDiff(old, next map[string]string) (added, changed, removed []string) {
	for k, v := range next {
		prev, ok := old[k]
		switch {
		case !ok:
			added = append(added, k)
		case prev != v:
			changed = append(changed, k)
		}
	}
	for k := range old {
		if _, ok := next[k]; !ok {
			removed = append(removed, k)
		}
	}
	sort.Strings(added)
	sort.Strings(changed)
	sort.Strings(removed)
	return added, changed, removed
}

// updateEvents describes what an app update changed, one event per concern.
// It carries key names and non-secret scalar from/to values only.
func updateEvents(name string, old, next store.DesiredService) []store.AppEvent {
	var events []store.AppEvent
	if added, changed, removed := envKeyDiff(old.Env, next.Env); len(added)+len(changed)+len(removed) > 0 {
		keys := slices.Concat(added, changed, removed)
		sort.Strings(keys)
		events = append(events, store.AppEvent{
			AppName: name, Kind: store.AppEventEnvChange, Keys: keys,
			Title:  keysTitle("Environment changed", keys),
			Detail: joinNonEmpty("; ", listPart("added", added), listPart("changed", changed), listPart("removed", removed)),
		})
	}
	if ev, ok := secretDeclarationEvent(name, old, next); ok {
		events = append(events, ev)
	}
	if old.Replicas != next.Replicas && next.Replicas > 0 {
		events = append(events, store.AppEvent{
			AppName: name, Kind: store.AppEventScale,
			Title: fmt.Sprintf("Scaled %d to %d replicas", max(old.Replicas, 1), next.Replicas),
		})
	}
	if ev, ok := configChangeEvent(name, old, next); ok {
		events = append(events, ev)
	}
	return events
}

func secretDeclarationEvent(name string, old, next store.DesiredService) (store.AppEvent, bool) {
	oldNames, nextNames := store.SecretEnvNames(old.SecretEnv), store.SecretEnvNames(next.SecretEnv)
	var added, removed []string
	for _, k := range nextNames {
		if !slices.Contains(oldNames, k) {
			added = append(added, k)
		}
	}
	for _, k := range oldNames {
		if !slices.Contains(nextNames, k) {
			removed = append(removed, k)
		}
	}
	if len(added)+len(removed) == 0 {
		return store.AppEvent{}, false
	}
	sort.Strings(added)
	sort.Strings(removed)
	keys := slices.Concat(added, removed)
	sort.Strings(keys)
	return store.AppEvent{
		AppName: name, Kind: store.AppEventSecretChange, Keys: keys,
		Title:  keysTitle("Secrets changed", keys),
		Detail: joinNonEmpty("; ", listPart("declared", added), listPart("undeclared", removed)),
	}, true
}

// configChangeEvent folds every changed setting into one event: scalar
// settings carry from and to, structured ones just their name.
func configChangeEvent(name string, old, next store.DesiredService) (store.AppEvent, bool) {
	var fields, details []string
	scalar := func(field, from, to string) {
		if from != to {
			fields = append(fields, field)
			details = append(details, fmt.Sprintf("%s %s to %s", field, from, to))
		}
	}
	structured := func(field string, changed bool) {
		if changed {
			fields = append(fields, field)
			details = append(details, field+" changed")
		}
	}
	scalar("port", fmt.Sprint(old.Port), fmt.Sprint(next.Port))
	scalar("host_port", hostPortText(old.HostPort), hostPortText(next.HostPort))
	scalar("bind_address", orDefault(old.BindAddress, store.DefaultBindAddress), orDefault(next.BindAddress, store.DefaultBindAddress))
	scalar("strategy", orDefault(old.Strategy, store.DefaultDeployStrategy), orDefault(next.Strategy, store.DefaultDeployStrategy))
	structured("resources", !jsonEqual(old.Resources, next.Resources))
	structured("health", !jsonEqual(old.Health, next.Health))
	structured("hooks", !jsonEqual(old.Hooks, next.Hooks))
	structured("command", !slices.Equal(old.Command, next.Command))
	structured("labels", !maps.Equal(old.Labels, next.Labels))
	if added, removed := sliceDiff(old.Domains, next.Domains); len(added)+len(removed) > 0 {
		fields = append(fields, "domains")
		details = append(details, "domains "+joinNonEmpty(" ", prefixAll("+", added), prefixAll("-", removed)))
	}
	if len(fields) == 0 {
		return store.AppEvent{}, false
	}
	return store.AppEvent{
		AppName: name, Kind: store.AppEventConfigChange, Keys: fields,
		Title:  "Config changed: " + strings.Join(fields, ", "),
		Detail: strings.Join(details, "; "),
	}, true
}

func hostPortText(p *int) string {
	if p == nil {
		return "auto"
	}
	return fmt.Sprint(*p)
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func jsonEqual(a, b any) bool {
	x, errA := json.Marshal(a)
	y, errB := json.Marshal(b)
	return errA == nil && errB == nil && string(x) == string(y)
}

func sliceDiff(old, next []string) (added, removed []string) {
	for _, v := range next {
		if !slices.Contains(old, v) {
			added = append(added, v)
		}
	}
	for _, v := range old {
		if !slices.Contains(next, v) {
			removed = append(removed, v)
		}
	}
	return added, removed
}

func prefixAll(prefix string, values []string) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = prefix + v
	}
	return strings.Join(parts, " ")
}

func listPart(label string, keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	return label + " " + strings.Join(keys, ", ")
}

func joinNonEmpty(sep string, parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, sep)
}
