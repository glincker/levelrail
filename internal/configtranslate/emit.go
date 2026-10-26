package configtranslate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/platformimport"
)

// EmitOptions narrow what a translation result becomes as stack documents.
type EmitOptions struct {
	// Services limits the emit to these service names, like apps create --service. Empty means all.
	Services []string
	// DBVersions sets the engine version per database engine, such as postgres=16.
	DBVersions map[string]string
}

// EmitInput converts the proposal into the neutral model that
// platformimport.EmitStack writes. Jobs, routes and build settings have no
// App document field yet, so they become notes instead of being dropped.
func (r Result) EmitInput(o EmitOptions) (platformimport.EmitInput, error) {
	in := platformimport.EmitInput{Source: "config files (" + formatNames(r.Detected) + ")"}
	want := map[string]bool{}
	for _, s := range o.Services {
		if _, ok := r.Fragment.Services[s]; !ok {
			return in, fmt.Errorf("no service %q in the proposal, have: %s", s, strings.Join(sortedKeys(r.Fragment.Services), ", "))
		}
		want[s] = true
	}
	generated := map[string]bool{}
	for _, f := range r.Findings {
		if f.Code == "heroku.env.generator" {
			generated[f.Subject] = true
		}
	}
	for _, name := range sortedKeys(r.Fragment.Services) {
		if len(want) > 0 && !want[name] {
			continue
		}
		in.Apps = append(in.Apps, emitService(name, r.Fragment.Services[name], generated, r.Fragment.Databases))
	}
	for _, name := range sortedKeys(r.Fragment.Databases) {
		db := r.Fragment.Databases[name]
		ver := o.DBVersions[db.Engine]
		if ver == "" {
			in.Notes = append(in.Notes, fmt.Sprintf("database %s (%s) has no version in the config files: pass --db-version %s=VERSION and emit again, or set it in databases.yaml", name, db.Engine, db.Engine))
		}
		in.Databases = append(in.Databases, platformimport.EmitDatabase{Name: name, Engine: db.Engine, Version: ver})
	}
	for _, f := range r.Findings {
		if f.Code == "config.source.unknown" {
			continue
		}
		n := fmt.Sprintf("[%s] %s: %s", f.Code, f.Source, f.Message)
		if f.Subject != "" {
			n = fmt.Sprintf("[%s] %s %s: %s", f.Code, f.Source, f.Subject, f.Message)
		}
		if f.Fix != "" {
			n += " Fix: " + f.Fix
		}
		in.Notes = append(in.Notes, n)
	}
	if len(r.Fragment.Databases) > 0 {
		in.Notes = append(in.Notes, "Databases are created empty. Copy the data in with `levelrail-cli migrate db-copy`.")
	}
	return in, nil
}

func formatNames(fs []Format) string {
	names := make([]string, len(fs))
	for i, f := range fs {
		names[i] = string(f)
	}
	return strings.Join(names, ", ")
}

func emitService(name string, svc Service, generated map[string]bool, dbs map[string]Database) platformimport.EmitApp {
	app := platformimport.EmitApp{Name: name, Port: svc.Port, Replicas: svc.Replicas, Command: svc.Command, Env: map[string]string{}}
	if svc.Resources != nil {
		app.Memory = svc.Resources.Memory
	}
	if svc.Health != nil && svc.Health.Readiness != nil {
		app.HealthPath = svc.Health.Readiness.Path
	}
	if svc.Hooks != nil {
		app.PreDeploy, app.PostDeploy = svc.Hooks.PreDeploy, svc.Hooks.PostDeploy
	}
	for _, k := range sortedKeys(svc.Env) {
		e := svc.Env[k]
		switch {
		case e.From != "":
			if db, field, ok := fromRef(e.From, dbs); ok {
				if app.Connections == nil {
					app.Connections = map[string]platformimport.EmitConnection{}
				}
				app.Connections[k] = platformimport.EmitConnection{Database: db, Field: field}
				continue
			}
			app.Secrets = append(app.Secrets, platformimport.EmitSecret{Name: k, Hint: "connection string of " + fromHint(e.From) + ", set it once the database exists or run databases link"})
		case e.Secret:
			hint := "required secret, set it before the first deploy"
			if generated[k] {
				hint = "the source generated this value: create a random one, for example openssl rand -hex 32"
			}
			app.Secrets = append(app.Secrets, platformimport.EmitSecret{Name: k, Hint: hint})
		default:
			app.Env[k] = e.Value
		}
	}
	switch svc.Build.Type {
	case "static":
		app.Notes = append(app.Notes, fmt.Sprintf("static site (build %q, publish %q): serve the built files from an image, apply cannot build it", svc.Build.Command, svc.Build.Publish))
	case "railpack", "dockerfile":
		app.Notes = append(app.Notes, "builds from source (type "+svc.Build.Type+"): build and push the image named in apps.yaml from your CI, or create the app with apps create --repo and --image-repo")
	}
	for _, j := range svc.Jobs {
		app.Jobs = append(app.Jobs, platformimport.EmitJob{Name: j.Name, Schedule: j.Schedule, Command: j.Command, Mode: j.Mode, Timezone: j.Timezone})
	}
	if len(svc.Routes) > 0 {
		app.Notes = append(app.Notes, fmt.Sprintf("%d route rule(s) (redirects and rewrites) are not in the App document", len(svc.Routes)))
	}
	if len(svc.Headers) > 0 {
		app.Notes = append(app.Notes, fmt.Sprintf("%d header rule(s) are not in the App document", len(svc.Headers)))
	}
	sort.Strings(app.Notes)
	return app
}

// fromRef resolves "postgres.main.url" against the emitted databases, so a
// reference only becomes a from: when its database is in the same stack.
func fromRef(from string, dbs map[string]Database) (db, field string, ok bool) {
	parts := strings.Split(from, ".")
	if len(parts) < 2 {
		return "", "", false
	}
	db, field = parts[len(parts)-2], parts[len(parts)-1]
	_, ok = dbs[db]
	return db, field, ok
}

// fromHint turns "postgres.main.url" into "postgres database main".
func fromHint(from string) string {
	parts := strings.Split(from, ".")
	if len(parts) == 3 {
		return parts[0] + " database " + parts[1]
	}
	return from
}
