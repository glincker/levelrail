package platformimport

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/envredact"
)

var connectionKeyRe = regexp.MustCompile(`(?i)^(database|redis|mongo|mongodb|db|postgres|postgresql|mysql)(_[a-z]+)?_(url|uri)$`)

// ConnectionHint is the SECRETS.md note for a variable that carries a database connection string.
const ConnectionHint = "database connection string, created empty here: set it after the database exists, or run databases link"

// EmitInput maps a Plan (any source adapter) to the neutral emit model. Secret
// values in the plan are dropped here, only their names go on.
func (p *Plan) EmitInput() EmitInput {
	in := EmitInput{Source: string(p.Report.Platform)}
	notes := map[string][]string{}
	for _, it := range p.Report.Items {
		switch {
		case it.Kind == "app" && it.Target != "":
			notes[it.Target] = append(notes[it.Target], it.Reasons...)
			for _, m := range it.Manual {
				notes[it.Target] = append(notes[it.Target], "next: "+m)
			}
		case it.Status == StatusUnsupported:
			n := fmt.Sprintf("%s %q was not emitted: %s", it.Kind, it.SourceName, strings.Join(it.Reasons, "; "))
			if len(it.Manual) > 0 {
				n += " (next: " + strings.Join(it.Manual, "; ") + ")"
			}
			in.Notes = append(in.Notes, n)
		}
	}
	in.Notes = append(in.Notes, p.Report.Notes...)
	for _, a := range p.Apps {
		app := EmitApp{Name: a.Name, Port: a.Port, Replicas: a.Replicas, Env: a.Env, Domains: a.Domains,
			MemoryBytes: a.MemoryBytes, NanoCPUs: a.NanoCPUs, Notes: notes[a.Name]}
		if a.Image != "" && !strings.HasSuffix(a.Image, ":pending") {
			app.Image = a.Image
		}
		if a.Health != nil {
			app.HealthPath = a.Health.Path
		}
		names := map[string]bool{}
		for k := range a.Secrets {
			names[k] = true
		}
		for _, k := range a.UnreadSecrets {
			names[k] = true
		}
		for _, k := range sortedMapKeys(names) {
			app.Secrets = append(app.Secrets, EmitSecret{Name: k, Hint: secretHint(k, a.UnreadSecrets)})
		}
		for _, v := range a.Volumes {
			dst := v.Name
			if dst == "" {
				dst = v.HostPath
			}
			app.Notes = append(app.Notes, fmt.Sprintf("volume %s mounted at %s is not in the documents: volumes are not supported by apply yet", dst, v.ContainerPath))
		}
		for _, c := range a.Crons {
			if commandMayHoldSecret(c.Command) {
				app.Notes = append(app.Notes, fmt.Sprintf("scheduled job %s (%s) is not in the documents: its command may hold a credential, add it in the app's Jobs tab", c.Name, c.Schedule))
				continue
			}
			app.Jobs = append(app.Jobs, EmitJob{Name: c.Name, Schedule: c.Schedule, Command: []string{"sh", "-c", c.Command}, Mode: "exec"})
		}
		app.Connections = matchConnections(names, a.Env, p.Databases)
		in.Apps = append(in.Apps, app)
	}
	for _, d := range p.Databases {
		in.Databases = append(in.Databases, EmitDatabase{Name: d.Name, Engine: d.Engine, Version: d.Version})
	}
	return in
}

// engineFamilies groups engines a connection variable prefix can mean.
var engineFamilies = map[string][]string{
	"postgres": {"postgres", "postgresql"}, "postgresql": {"postgres", "postgresql"},
	"mysql": {"mysql", "mariadb"}, "redis": {"redis", "valkey", "keydb", "dragonfly"},
	"mongo": {"mongodb", "mongo"}, "mongodb": {"mongodb", "mongo"},
	"database": {"postgres", "postgresql", "mysql", "mariadb"}, "db": {"postgres", "postgresql", "mysql", "mariadb"},
}

// matchConnections points each connection-named variable at the one emitted
// database of its engine. An ambiguous or missing match stays a secret name.
func matchConnections(secretNames map[string]bool, env map[string]string, dbs []DatabasePlan) map[string]EmitConnection {
	candidates := sortedMapKeys(secretNames)
	for k := range env {
		if !secretNames[k] {
			candidates = append(candidates, k)
		}
	}
	out := map[string]EmitConnection{}
	for _, k := range candidates {
		m := connectionKeyRe.FindStringSubmatch(k)
		if m == nil {
			continue
		}
		var hit []string
		for _, d := range dbs {
			for _, e := range engineFamilies[strings.ToLower(m[1])] {
				if d.Engine == e {
					hit = append(hit, d.Name)
				}
			}
		}
		if len(hit) == 1 {
			out[k] = EmitConnection{Database: hit[0], Field: "url"}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func secretHint(name string, unread []string) string {
	hint := "secret on the source, the value was not copied"
	if connectionKeyRe.MatchString(name) {
		hint = ConnectionHint
	}
	for _, u := range unread {
		if u == name {
			return hint + "; the source would not reveal its value"
		}
	}
	return hint
}

var credentialFlagRe = regexp.MustCompile(`(?i)(authorization:|bearer\s|\s-u\s+\S+:|--password|--token|://[^/\s:@]+:[^@\s]+@)`)

// commandMayHoldSecret errs toward keeping a cron command out of git: the
// emitted files are meant for a pull request.
func commandMayHoldSecret(cmd string) bool {
	if envredact.SensitiveValue("COMMAND", cmd) || credentialFlagRe.MatchString(cmd) {
		return true
	}
	for _, tok := range strings.Fields(cmd) {
		k, v, ok := strings.Cut(strings.Trim(tok, `'"`), "=")
		if envredact.SensitiveValue("COMMAND", tok) || ok && v != "" && (envredact.SensitiveName(k) || envredact.SensitiveValue(k, v)) {
			return true
		}
	}
	return false
}

func sortedMapKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
