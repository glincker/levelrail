package coolifyexit

import (
	"fmt"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/platformimport"
)

// buildWaves orders apps so the least risky DNS switches go first: stateless apps,
// then apps with one kind of state, then apps with both a volume and a database.
func buildWaves(d *platformimport.Discovery, plan *platformimport.Plan) []Wave {
	skipped := map[string]bool{}
	for _, it := range plan.Report.Items {
		if it.Kind == "app" && (it.Status == platformimport.StatusUnsupported || it.Status == platformimport.StatusSkipped) {
			skipped[it.SourceID] = true
		}
	}
	titles := []struct{ title, why string }{
		{"Stateless apps", "no volume and no database: a bad switch loses nothing and rolls back by restoring the old record"},
		{"Apps with one kind of state", "a volume or a database: pause writes on the old app, run the last data copy, then switch"},
		{"Apps with a volume and a database", "most state to keep consistent: switch last, one at a time, with writes paused on the old app"},
	}
	waves := make([]Wave, len(titles))
	for i, t := range titles {
		waves[i] = Wave{Number: i + 1, Title: t.title, Why: t.why, Apps: []WaveApp{}}
	}
	for _, a := range d.Apps {
		if skipped[a.SourceID] || !inPlan(plan, a.SourceID) {
			continue
		}
		state := 0
		if len(a.Volumes) > 0 {
			state++
		}
		if referencesDatabase(a, d.Databases) {
			state++
		}
		name := a.Name
		for _, it := range plan.Report.Items {
			if it.Kind == "app" && it.SourceID == a.SourceID && it.Target != "" {
				name = it.Target
			}
		}
		waves[state].Apps = append(waves[state].Apps, WaveApp{Name: name, Domains: a.Domains})
	}
	out := waves[:0]
	for _, w := range waves {
		sort.Slice(w.Apps, func(i, j int) bool { return w.Apps[i].Name < w.Apps[j].Name })
		if len(w.Apps) > 0 {
			out = append(out, w)
		}
	}
	return out
}

func inPlan(plan *platformimport.Plan, id string) bool {
	for _, it := range plan.Report.Items {
		if it.Kind == "app" && it.SourceID == id {
			return true
		}
	}
	return false
}

func referencesDatabase(a platformimport.App, dbs []platformimport.Database) bool {
	for _, e := range a.Env {
		for _, db := range dbs {
			if db.InternalHost != "" && strings.Contains(e.Value, db.InternalHost) {
				return true
			}
		}
	}
	return false
}

func buildChecklist(r Report, d *platformimport.Discovery) []Step {
	manual := r.Counts[Manual] + r.Counts[Skipped]
	steps := []Step{
		{ID: "backup-source", Title: "Back up Coolify itself",
			Detail: "Copy the source .env (it holds the APP_KEY that decrypts everything) and take a database dump of Coolify's own Postgres. Keep both until the last step. Nothing here changes the source.",
		},
		{ID: "resolve-manual", Title: "Clear the manual items",
			Detail:   fmt.Sprintf("%d resource(s) are skipped or need a person. Work through the rows marked manual or skipped before staging; skipped resources never move on their own.", manual),
			Commands: []string{"migrate coolify --url URL --token TOKEN --dry-run"},
		},
		{ID: "stage", Title: "Stage everything stopped and unrouted",
			Detail:   "Apps and databases are created here with no domains attached. Coolify keeps serving. Re-running skips what exists.",
			Commands: []string{"import apps --from coolify --url URL --apply"},
		},
		{ID: "copy-data", Title: "Copy database and volume data",
			Detail:   "Copy while the old apps still run. Dumps only read the source. Repeat the copy right before each switch to shrink the final window.",
			Commands: []string{"migrate db-copy NAME --host HOST --user USER --database DB", "migrate volumes --source user@host"},
		},
		{ID: "verify-staged", Title: "Verify before any DNS change",
			Detail:   "Build or pull each staged app, wait for its health check, then test through the real name without touching DNS: curl --resolve app.example.com:443:NEW_IP https://app.example.com/healthz. Re-run this report: resources that exist here show as imported.",
			Commands: []string{"import apps --session ID --verify", "migrate coolify --url URL --token TOKEN --dry-run"},
		},
		{ID: "lower-ttl", Title: "Lower DNS TTL ahead of time",
			Detail: "Set the TTL on every record you will switch to its lowest value at least one old TTL before the first switch, so a rollback takes effect quickly.",
		},
		{ID: "recreate-hooks", Title: "Recreate webhooks and enable jobs",
			Detail: "Add each git webhook on the git host pointing here, then enable the scheduled jobs of an app only when its traffic moves, so a job never runs on both hosts.",
		},
	}
	for _, w := range r.Waves {
		var apps []string
		for _, a := range w.Apps {
			apps = append(apps, a.Name)
		}
		steps = append(steps, Step{ID: fmt.Sprintf("switch-wave-%d", w.Number), Title: fmt.Sprintf("Switch DNS, wave %d: %s", w.Number, strings.ToLower(w.Title)),
			Detail:   w.Why + ". Apps: " + strings.Join(apps, ", ") + ". Switch one app, check it, then the next.",
			Commands: []string{"import cutover plan --session ID --app NAME", "import cutover run --session ID --app NAME --dry-run", "import cutover run --session ID --app NAME --confirm NAME"},
		})
	}
	steps = append(steps,
		Step{ID: "keep-fallback", Title: "Keep Coolify as the fallback until verified",
			Detail:   "Do not stop or delete anything on the source. A rollback restores the previous DNS value and the old app answers again. For an app with a database, the fallback is only valid until the first write lands here: after that, copy the data back before rolling back.",
			Commands: []string{"import cutover rollback --session ID --app NAME"},
		},
		Step{ID: "retire", Title: "Retire Coolify",
			Detail:   "After a full traffic cycle with backups and jobs run at least once here, restore one backup as a test, download the receipt, then stop Coolify. Keep its backup files and the saved .env.",
			Commands: []string{"import apps --session ID --receipt"},
		},
	)
	if len(d.Servers) > 1 {
		steps[1].Detail += " The source has more than one server, so plan which node each app should land on."
	}
	return steps
}
