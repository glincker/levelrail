package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/glincker/levelrail/internal/store"
)

// dangledAppsHeader lists the visible apps whose references were invalidated by a force delete.
const dangledAppsHeader = "X-Dangling-Apps"

// databaseInUseMessage names only the apps the caller can see and counts the rest.
func (rt *Router) databaseInUseMessage(w http.ResponseWriter, r *http.Request, users []string) (string, bool) {
	canSee, err := rt.appVisibilityFilter(r)
	if err != nil {
		rt.internalError(w, "api: delete database: app visibility", err)
		return "", false
	}
	var parts []string
	hidden := 0
	for _, u := range users {
		if canSee(u) {
			parts = append(parts, u)
		} else {
			hidden++
		}
	}
	if hidden > 0 {
		parts = append(parts, strconv.Itoa(hidden)+" more not visible to you")
	}
	return "database is used by apps (" + strings.Join(parts, ", ") + "); detach them or retry with force=true", true
}

// invalidateDatabaseRefs marks every reference to a removed database as dangling, so a later
// database reusing the name is never bound to them. Visible apps are listed in a response header.
func (rt *Router) invalidateDatabaseRefs(ctx context.Context, w http.ResponseWriter, r *http.Request, dbName string) {
	ctx = context.WithoutCancel(ctx)
	users, err := rt.appsUsingDatabase(ctx, dbName)
	if err != nil || len(users) == 0 {
		return
	}
	var visible []string
	canSee, visErr := rt.appVisibilityFilter(r)
	for _, app := range users {
		if err := rt.dangleAppDatabaseRefs(ctx, app, dbName); err != nil {
			rt.logger.Error("api: delete database: invalidate refs failed", slog.String("app", app), slog.String("database", dbName), slog.String("error", err.Error()))
			continue
		}
		if visErr == nil && canSee(app) {
			visible = append(visible, app)
		}
	}
	if len(visible) > 0 {
		w.Header().Set(dangledAppsHeader, strings.Join(visible, ","))
	}
}

func (rt *Router) dangleAppDatabaseRefs(ctx context.Context, app, dbName string) error {
	svc, err := rt.apps.GetDesiredService(ctx, app)
	if err != nil || svc == nil {
		return err
	}
	for envVar, ref := range svc.DatabaseEnv {
		if ref.Database != dbName {
			continue
		}
		ref.Instance = store.DanglingDatabaseInstance
		if err := rt.apps.SetServiceDatabaseEnvVar(ctx, app, envVar, &ref); err != nil {
			return err
		}
	}
	if att := svc.DatabaseAttachment; att != nil && att.DatabaseName == dbName {
		dangling := *att
		dangling.InstanceID = store.DanglingDatabaseInstance
		return rt.apps.UpdateServiceDatabaseAttachment(ctx, app, &dangling)
	}
	return nil
}
