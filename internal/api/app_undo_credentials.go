package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Categories a soft delete destroys and an undo cannot bring back.
const (
	undoGitSource    = "git_source"
	undoLogDrain     = "log_drain"
	undoIntegrations = "integrations"
	undoBuildHooks   = "build_hooks"

	// undoNeedsLabel rides in the undo snapshot's labels so a restore knows what was lost.
	undoNeedsLabel = "restore-needs-attention"

	kindRestoreNeeds = "restore_needs_attention"
)

type restoreAttentionItem struct {
	Category string `json:"category"`
	Fix      string `json:"fix"`
}

// undoLostCategories lists the categories this app has rows for and a soft delete will not park.
func (rt *Router) undoLostCategories(ctx context.Context, svc store.DesiredService) []string {
	var out []string
	if rt.gitSources != nil {
		if _, err := rt.gitSources.GetGitSource(ctx, svc.Name); err == nil {
			out = append(out, undoGitSource)
		}
	}
	if svc.LogDrain != nil {
		out = append(out, undoLogDrain)
	}
	if rt.appIntegrations != nil {
		if rows, err := rt.appIntegrations.ListAppIntegrationsForService(ctx, svc.Name); err == nil && len(rows) > 0 {
			out = append(out, undoIntegrations)
		}
	}
	if rt.buildHooks != nil {
		if rows, err := rt.buildHooks.ListBuildHooks(ctx, svc.Name); err == nil && len(rows) > 0 {
			out = append(out, undoBuildHooks)
		}
	}
	return out
}

// undoCategoriesFromSnapshot reads the categories recorded at delete time.
func undoCategoriesFromSnapshot(snapshot string) []string {
	var svc store.DesiredService
	if json.Unmarshal([]byte(snapshot), &svc) != nil {
		return nil
	}
	return splitUndoLabel(svc.Labels[undoNeedsLabel])
}

func splitUndoLabel(v string) []string {
	if v == "" {
		return nil
	}
	return strings.Split(v, ".")
}

func (rt *Router) undoFixCommand(category, app string) string {
	cli := rt.cliName()
	switch category {
	case undoGitSource:
		return cli + " apps git-source connect " + app
	case undoLogDrain:
		return cli + " apps log-drain set " + app
	case undoIntegrations:
		return cli + " apps integrations attach " + app
	case undoBuildHooks:
		return cli + " apps build-hooks create " + app
	}
	return ""
}

func (rt *Router) restoreAttentionItems(app string, categories []string) []restoreAttentionItem {
	out := make([]restoreAttentionItem, 0, len(categories))
	for _, c := range categories {
		out = append(out, restoreAttentionItem{Category: c, Fix: rt.undoFixCommand(c, app)})
	}
	return out
}

// undoCategoryMissing reports whether a restored app still lacks what the category stands for.
func (rt *Router) undoCategoryMissing(ctx context.Context, app, category string) bool {
	switch category {
	case undoGitSource:
		if rt.gitSources == nil {
			return false
		}
		_, err := rt.gitSources.GetGitSource(ctx, app)
		return err != nil
	case undoLogDrain:
		if rt.secrets == nil {
			return false
		}
		keys, err := rt.secrets.ListKeys(ctx, store.LogDrainSecretService(app))
		return err == nil && len(keys) == 0
	case undoIntegrations:
		if rt.appIntegrations == nil {
			return false
		}
		rows, err := rt.appIntegrations.ListAppIntegrationsForService(ctx, app)
		return err == nil && len(rows) == 0
	case undoBuildHooks:
		if rt.buildHooks == nil {
			return false
		}
		rows, err := rt.buildHooks.ListBuildHooks(ctx, app)
		return err == nil && len(rows) == 0
	}
	return false
}

// restoreNeedsAttentionItems raises one feed item per category a restored app still lacks.
func (rt *Router) restoreNeedsAttentionItems(r *http.Request, abilities []string) []attention.Item {
	if !hasAbility(abilities, AbilityRead) {
		return nil
	}
	ctx := r.Context()
	apps, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil
	}
	var items []attention.Item
	for _, svc := range apps {
		for _, c := range splitUndoLabel(svc.Labels[undoNeedsLabel]) {
			if !rt.undoCategoryMissing(ctx, svc.Name, c) {
				continue
			}
			subject := svc.Name + "/" + c
			it := feedItem(attention.Warning, kindRestoreNeeds, subject, "Not restored by undo: "+c, map[string]string{"app": svc.Name, "category": c})
			it.Title = svc.Name + " lost its " + strings.ReplaceAll(c, "_", " ") + " when it was deleted"
			it.Action = rt.undoFixCommand(c, svc.Name)
			it.Link = "/apps/" + svc.Name
			it.Class = attention.ClassRecommendation
			items = append(items, it)
		}
	}
	return items
}
