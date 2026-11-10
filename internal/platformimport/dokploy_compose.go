package platformimport

import (
	"context"
	"net/url"
	"path"
	"strings"
)

// dokployCompose is a compose stack as compose.one returns it.
type dokployCompose struct {
	ComposeID   string          `json:"composeId"`
	Name        string          `json:"name"`
	AppName     string          `json:"appName"`
	SourceType  string          `json:"sourceType"`
	ComposeFile string          `json:"composeFile"`
	Env         string          `json:"env"`
	Domains     []dokployDomain `json:"domains"`
	Mounts      []dokployMount  `json:"mounts"`
}

// readCompose reads one stack; a stack whose file lives in git is reported blocked, nothing is fetched.
func (s *DokploySource) readCompose(ctx context.Context, project string, ref dokployRef) ComposeApp {
	var c dokployCompose
	if err := s.r.get(ctx, "/api/compose.one", url.Values{"composeId": {ref.ID}}, &c); err != nil {
		return ComposeApp{SourceID: ref.ID, Name: ref.Name, Project: project, Verdict: ComposeBlocked,
			Blocked: []string{"could not read the compose stack: " + err.Error()}}
	}
	return mapDokployCompose(project, c)
}

// mapDokployCompose turns a stack into a compose app with its env split by secret flag.
func mapDokployCompose(project string, c dokployCompose) ComposeApp {
	name := c.Name
	if name == "" {
		name = c.AppName
	}
	hosts := make([]string, 0, len(c.Domains))
	for _, d := range c.Domains {
		hosts = append(hosts, d.Host)
	}
	body := c.ComposeFile
	src := strings.ToLower(c.SourceType)
	if strings.TrimSpace(body) == "" && src != "" && src != "raw" {
		return ComposeApp{SourceID: c.ComposeID, Name: name, Project: project, Verdict: ComposeBlocked,
			Blocked: []string{"the compose file lives in a " + c.SourceType + " repository, which the import does not clone; deploy it from git with the compose deploy command"}}
	}
	out := NewComposeApp(c.ComposeID, name, project, body, parseEnvBlob(c.Env), hosts)
	for _, m := range c.Mounts {
		if m.Type != "file" {
			continue
		}
		out.Notes = append(out.Notes, Note{Area: "file", Reason: "file mount " + path.Clean("/"+m.MountPath) + " of the stack is not copied", Manual: "declare it under configs: in the compose file"})
	}
	return out
}

// dokployFileMount copies a file mount into the app; a refused path or size becomes a note.
func dokployFileMount(app *App, m dokployMount) []Note {
	f, err := NewAppFile(m.MountPath, []byte(m.Content))
	if err != nil {
		return []Note{{Area: "file", Reason: "file mount was not imported: " + err.Error(), Manual: "recreate the file with the app files command"}}
	}
	app.Files = append(app.Files, f)
	return []Note{{Area: "file", Reason: "file mount " + f.Path + " is copied only by the migrate command", Manual: "set it with the app files command"}}
}
