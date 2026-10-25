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
			app.Notes = append(app.Notes, fmt.Sprintf("scheduled job %s (%s) is not in the documents: %s", c.Name, c.Schedule, cronCommand(c.Command)))
		}
		in.Apps = append(in.Apps, app)
	}
	for _, d := range p.Databases {
		in.Databases = append(in.Databases, EmitDatabase{Name: d.Name, Engine: d.Engine, Version: d.Version})
	}
	return in
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

func cronCommand(cmd string) string {
	if envredact.SensitiveValue("COMMAND", cmd) {
		return "command omitted, it may hold a credential"
	}
	return "`" + cmd + "`"
}

func sortedMapKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
