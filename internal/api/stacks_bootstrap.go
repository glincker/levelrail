package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/store"
)

// stackFileDocs parses the request files; parse problems surface later from build.
func stackFileDocs(req stackRequest) []*iac.Document {
	sources := make([]iac.Source, len(req.Files))
	for i, f := range req.Files {
		sources[i] = iac.Source{Name: f.Name, Data: []byte(f.Content)}
	}
	docs, issues := iac.ParseDocuments(sources)
	if len(issues) > 0 {
		return nil
	}
	return docs
}

// newStackProject names the project a new stack lands in: the request's own,
// else the single Project document in the files. declared says the files hold it.
func newStackProject(req stackRequest) (name string, declared bool, err error) {
	var projects []string
	for _, d := range stackFileDocs(req) {
		if d.Kind == iac.KindProject {
			projects = append(projects, d.Name)
		}
	}
	sort.Strings(projects)
	if req.Project != "" {
		for _, p := range projects {
			if p == req.Project {
				return req.Project, true, nil
			}
		}
		return req.Project, false, nil
	}
	switch len(projects) {
	case 1:
		return projects[0], true, nil
	case 0:
		return "", false, fmt.Errorf("the files name no project: add a Project document, or pass the project the stack belongs to")
	}
	return "", false, fmt.Errorf("the files hold several Project documents (%s): pass the project this stack belongs to", strings.Join(projects, ", "))
}

func missingProjectMessage(project string) string {
	return fmt.Sprintf("project %q does not exist and this caller cannot create it; create it first (POST /api/v1/projects with {\"name\":%q}, or the CLI 'apps projects create --name %s'), then apply again", project, project, project)
}

// mayCreateProject mirrors the direct project create: the write ability, never a
// CI or stack-scoped token, whose reach ends at its own stack.
func (rt *Router) mayCreateProject(r *http.Request) bool {
	c := rt.stackCaller(r)
	if c.isCI {
		return false
	}
	_, _, abilities, err := rt.callerPrincipal(r)
	if err != nil || !hasAbility(abilities, AbilityWrite) {
		return false
	}
	if c.kind == store.PrincipalTypeToken {
		if ts := rt.tokenStackScopes(); ts != nil {
			sc, err := ts.GetAPITokenStackScope(r.Context(), c.id)
			if err != nil || sc != nil {
				return false
			}
		}
	}
	return true
}

// createProjectForStack makes the project a first apply declared. It writes the response on failure.
func (rt *Router) createProjectForStack(w http.ResponseWriter, r *http.Request, name string) (store.Project, bool) {
	team, ok := rt.resolveProjectTeam(w, r, "")
	if !ok {
		return store.Project{}, false
	}
	id, err := randomProjectID()
	if err != nil {
		rt.internalError(w, "api: stack create project: generate id failed", err)
		return store.Project{}, false
	}
	p := store.Project{ID: id, Name: name, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := rt.projects.SaveProject(r.Context(), p); err != nil {
		rt.internalError(w, "api: stack create project failed", err, slog.String("id", id))
		return store.Project{}, false
	}
	if err := rt.organizations.SetProjectOrganization(r.Context(), id, team); err != nil {
		rt.internalError(w, "api: stack create project: assign team failed", err, slog.String("project_id", id))
		return store.Project{}, false
	}
	p.OrgID = team
	rt.auditStack(r, auditStackCreate, map[string]any{"project": name, "created_with_stack": true})
	return p, true
}

// emptyStackReason explains why the files plan to zero resources, naming what the project filter dropped.
func emptyStackReason(req stackRequest, project string) string {
	docs := stackFileDocs(req)
	if len(docs) == 0 {
		return "the files hold no resource documents, so there is nothing to create"
	}
	kept := map[*iac.Document]bool{}
	for _, d := range iac.FilterProject(docs, project) {
		kept[d] = true
	}
	var dropped []string
	for _, d := range docs {
		if !kept[d] {
			dropped = append(dropped, string(d.Kind)+"/"+d.Name)
		}
	}
	if len(dropped) == 0 {
		return "the files plan to zero resources, so there is nothing to create"
	}
	return fmt.Sprintf("the plan is empty: the project filter %q dropped %d of %d documents (%s); name the right project or fix their spec.project", project, len(dropped), len(docs), strings.Join(dropped, ", "))
}

// resolveNewStack decides project and emptiness for a stack that does not exist yet.
// It answers the HTTP error itself and reports whether the project must be created.
func (rt *Router) resolveNewStack(w http.ResponseWriter, r *http.Request, req stackRequest) (project string, create, ok bool) {
	project, declared, err := newStackProject(req)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return "", false, false
	}
	if _, found := rt.projectByName(r.Context(), project); !found {
		if !declared || !rt.mayCreateProject(r) {
			writeError(w, http.StatusUnprocessableEntity, missingProjectMessage(project))
			return "", false, false
		}
		create = true
	}
	return project, create, true
}
