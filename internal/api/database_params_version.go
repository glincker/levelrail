package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/dbparams"
	"github.com/GLINCKER/levelrail/internal/store"
)

// AuditActionDatabaseParamsDropIncompatible records parameters removed on an
// explicit drop_incompatible_params version change.
const AuditActionDatabaseParamsDropIncompatible = "database.params.drop_incompatible"

type databaseParamsDropper interface {
	DropDatabaseEngineParams(ctx context.Context, name string, expected, keep map[string]string, audit store.AuditEntry) error
	SetDatabaseVersionDroppingEngineParams(ctx context.Context, name, fromVersion, toVersion string, expected, keep map[string]string, audit store.AuditEntry) error
	SetDatabaseVersionIfEngineParams(ctx context.Context, name, fromVersion, toVersion string, expected map[string]string) error
}

// paramsPendingHeader tells a version change caller that a staged parameter
// set exists and still needs an explicit params apply.
const paramsPendingHeader = "Levelrail-Params-Pending"

// versionParamsPlan is what a version change does to the running parameters.
// It is computed before anything is written.
type versionParamsPlan struct {
	pending bool
	applied map[string]string
	keep    map[string]string
	dropped map[string]string
	// checked is true when the running set was read and re-validated.
	checked bool
}

func (p versionParamsPlan) drops() bool { return len(p.dropped) > 0 }

// planParamsForVersion re-validates the running set against the target
// version with the staging validators and writes nothing. It answers 409
// listing the refused names unless drop is set and the store can drop.
func (rt *Router) planParamsForVersion(w http.ResponseWriter, r *http.Request, d *store.DesiredDatabase, version string, drop bool) (versionParamsPlan, bool) {
	s, isParams := rt.databases.(databaseParamsStore)
	if !isParams {
		return versionParamsPlan{}, true
	}
	p, err := s.GetDatabaseEngineParams(r.Context(), d.Name)
	if err != nil {
		rt.internalError(w, "api: load database params for version change failed", err, slog.String("name", d.Name))
		return versionParamsPlan{}, false
	}
	plan := versionParamsPlan{pending: p.Pending != nil, applied: p.Applied, checked: true}
	bad := dbparams.Incompatible(d.Engine, version, p.Applied, rt.paramsLimits(r.Context(), d))
	if len(bad) == 0 {
		return plan, true
	}
	if _, canDrop := rt.databases.(databaseParamsDropper); !drop || !canDrop {
		writeError(w, http.StatusConflict, fmt.Sprintf("engine parameters %s are not valid on version %s: unset them and apply first, or retry with drop_incompatible_params", strings.Join(bad, ", "), version))
		return versionParamsPlan{}, false
	}
	plan.keep = make(map[string]string, len(p.Applied))
	plan.dropped = make(map[string]string, len(bad))
	for k, v := range p.Applied {
		plan.keep[k] = v
	}
	for _, k := range bad {
		plan.dropped[k] = plan.keep[k]
		delete(plan.keep, k)
	}
	return plan, true
}

// dropAudit is the audit row of a planned drop, written with the drop itself.
func (rt *Router) dropAudit(r *http.Request, method, name string, plan versionParamsPlan) (store.AuditEntry, bool) {
	return rt.paramsAuditEntry(r, AuditActionDatabaseParamsDropIncompatible, AbilityWriteSensitive, method, name, plan.dropped)
}

// stagedParamsIncompatible lists staged names the current version refuses,
// so an apply never promotes what could not be staged now.
func (rt *Router) stagedParamsIncompatible(ctx context.Context, d *store.DesiredDatabase, pending map[string]string) []string {
	return dbparams.Incompatible(d.Engine, d.Version, pending, rt.paramsLimits(ctx, d))
}
