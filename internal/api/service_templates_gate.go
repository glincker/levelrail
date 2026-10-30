package api

import (
	"context"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/store"
)

// templateGates maps a catalog template ID to the experimental flag it needs.
// The static catalog keeps the entry (docs and generators read it); only the
// API listing, lookup and install hide it while the flag is off.
var templateGates = map[string]experimental.Feature{
	"mail-mox": experimental.Mail,
}

// templateAvailable reports whether a catalog template may be listed or installed now.
func templateAvailable(id string) bool {
	f, gated := templateGates[id]
	return !gated || experimental.Enabled(f)
}

// availableTemplates is catalog.Templates minus the gated entries that are off.
func availableTemplates() []catalog.Template {
	out := make([]catalog.Template, 0, len(catalog.Templates))
	for _, tpl := range catalog.Templates {
		if templateAvailable(tpl.ID) {
			out = append(out, tpl)
		}
	}
	return out
}

// instanceOnlyTemplates are trusted by the control plane once installed (mail
// receives admin calls and new mailbox passwords), so a tenant may not own one.
var instanceOnlyTemplates = map[string]bool{
	mailTemplateID: true,
}

const msgTemplateInstanceOnly = "only an instance admin can install the mail server"

// callerIsInstance reports whether the team gate resolved an instance level
// caller. Outside a gated request it is false, so callers fail closed.
func callerIsInstance(r *http.Request) bool {
	scope, ok := callerTeamScope(r.Context())
	return ok && (scope == nil || scope.instance)
}

// instanceOwnedApp reports whether an app belongs to the instance (no team
// owner, or the default team). Any lookup error counts as not owned.
func (rt *Router) instanceOwnedApp(ctx context.Context, name string) bool {
	owner, err := rt.ownerForResource(ctx, resourcePrefixApp, name)
	return err == nil && (owner == nil || owner.TeamID == "" || owner.TeamID == store.DefaultTeamID)
}

// isInstanceMailApp is the one test for "this app is the instance mail server".
func (rt *Router) isInstanceMailApp(ctx context.Context, s store.DesiredService) bool {
	return s.TemplateID == mailTemplateID && rt.instanceOwnedApp(ctx, s.Name)
}
