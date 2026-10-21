package api

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/kit/semver"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

// Install states reported by the update preflight and release list.
const (
	UpdateStateUpgradeAvailable = "upgrade_available"
	UpdateStateUpToDate         = "up_to_date"
	UpdateStateAheadOfReleases  = "ahead_of_releases"
	UpdateStateBlockedOlder     = "blocked_older"
	UpdateStateUnknown          = "unknown"
)

const (
	envUpgradeVerifyWindow     = "APP_UPGRADE_VERIFY_WINDOW"
	defaultUpgradeVerifyWindow = 3 * time.Minute
)

var devSuffixRe = regexp.MustCompile(`-g[0-9a-f]{6,}$|-\d+-g[0-9a-f]+`)

// isDevBuild reports a version string that is not a clean release tag.
func isDevBuild(v string) bool {
	if strings.Contains(v, "+") || strings.Contains(v, "-dev") || devSuffixRe.MatchString(v) {
		return true
	}
	_, ok := semver.Compare(v, v)
	return !ok
}

type installStateInput struct {
	Current         string
	Latest          string
	UpdateAvailable bool
	DBSchema        int
	LatestSchema    int
}

type installState struct {
	State  string
	Reason string
	// Migrations is the schema delta an upgrade applies, -1 when unknown.
	Migrations int
}

// classifyInstall names the install's relationship to the newest release.
// Schemas are -1 when unknown. The engine upgrades straight to one target
// release, so there is no needs-intermediate-step state.
func classifyInstall(in installStateInput) installState {
	known := in.DBSchema >= 0 && in.LatestSchema >= 0
	switch {
	case in.Latest == "":
		return installState{State: UpdateStateUnknown, Reason: "The latest release could not be fetched.", Migrations: -1}
	case known && in.LatestSchema < in.DBSchema:
		return installState{State: UpdateStateAheadOfReleases, Migrations: -1, Reason: fmt.Sprintf(
			"Database is at schema %d, the latest release %s supports %d.", in.DBSchema, in.Latest, in.LatestSchema)}
	case in.UpdateAvailable:
		delta := -1
		if known {
			delta = in.LatestSchema - in.DBSchema
		}
		return installState{State: UpdateStateUpgradeAvailable, Migrations: delta, Reason: fmt.Sprintf("%s is newer and compatible with your database.", in.Latest)}
	case isDevBuild(in.Current):
		return installState{State: UpdateStateAheadOfReleases, Migrations: -1, Reason: fmt.Sprintf(
			"Running a development build (%s); the latest release is %s.", in.Current, in.Latest)}
	default:
		return installState{State: UpdateStateUpToDate, Migrations: 0, Reason: "You are on the latest release."}
	}
}

// releaseState classifies one listed release against the database schema.
func releaseState(dbSchema, schema int, running bool) string {
	switch {
	case running:
		return UpdateStateUpToDate
	case dbSchema >= 0 && schema >= 0 && schema < dbSchema:
		return UpdateStateBlockedOlder
	default:
		return UpdateStateUnknown
	}
}

func upgradeVerifyWindow() time.Duration {
	if d, err := time.ParseDuration(os.Getenv(envUpgradeVerifyWindow)); err == nil && d > 0 {
		return d
	}
	return defaultUpgradeVerifyWindow
}

// fillInstallState sets the state fields on the preflight. It resolves the
// target's schema itself so an up-to-date install still reports it.
func (rt *Router) fillInstallState(ctx context.Context, out *updatePreflightResource, tag string, updateAvailable bool, assets []string) {
	latestSchema := -1
	if tag != "" && out.ReleaseKind != upgrade.KindEdgeImage {
		if in := rt.schemaInputFor(ctx, tag, assets); in != nil && in.State == upgrade.SchemaKnown {
			latestSchema = in.Release
		}
	}
	dbSchema := rt.liveSchemaVersion(ctx)
	st := classifyInstall(installStateInput{
		Current: out.CurrentVersion, Latest: tag, UpdateAvailable: updateAvailable,
		DBSchema: dbSchema, LatestSchema: latestSchema,
	})
	out.State, out.StateReason = st.State, st.Reason
	out.DBSchema, out.LatestSchema = optInt(dbSchema), optInt(latestSchema)
	out.MigrationsToApply = optInt(st.Migrations)
	out.RestorePointFirst = st.State == UpdateStateUpgradeAvailable
	out.VerifyWindowSeconds = int(upgradeVerifyWindow().Seconds())
	for v := range rt.retainedByVersion() {
		if v == out.CurrentVersion {
			continue
		}
		if out.RollbackVersion == "" {
			out.RollbackVersion = v
			continue
		}
		if c, ok := semver.Compare(v, out.RollbackVersion); ok && c > 0 {
			out.RollbackVersion = v
		}
	}
}
