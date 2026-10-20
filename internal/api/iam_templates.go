package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// policyTemplatesVersion changes whenever a template document changes.
const policyTemplatesVersion = 1

const (
	templateParamEnvironment = "environment"
	templateParamApp         = "app"
	templateParamDatabase    = "database"
	templateParamProject     = "project"
	// projectEnvironmentsSuffix is the derived param a project expands into.
	projectEnvironmentsSuffix = ".environments"
)

var nonProductionKindResources = []string{
	resourcePrefixEnvironmentKind + "dev",
	resourcePrefixEnvironmentKind + "test",
	resourcePrefixEnvironmentKind + "uat",
	resourcePrefixEnvironmentKind + "preview",
}

const productionKindResource = resourcePrefixEnvironmentKind + "production"

var templateParamPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

type policyTemplateParam struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	// Kind tells a picker what to offer: environment, app, database or project.
	Kind string `json:"kind,omitempty"`
}

// policyTemplate is one ready-made policy; build renders its document.
type policyTemplate struct {
	ID          string
	Name        string
	Description string
	Params      []policyTemplateParam
	build       func(params map[string]string) Document
}

func stmt(effect Effect, actions []string, resources ...string) Statement {
	return Statement{Effect: effect, Action: actions, Resource: resources}
}

var mutatingAbilities = []string{AbilityWrite, AbilityWriteSensitive, AbilityDeploy, AbilityRoot}

var policyTemplates = []policyTemplate{
	{
		ID:          "read-only",
		Name:        "Read only",
		Description: "Can see everything and change nothing. Any write, deploy or root request is denied.",
		build: func(map[string]string) Document {
			return Document{Statement: []Statement{
				stmt(EffectAllow, []string{AbilityRead}, "*"),
				stmt(EffectDeny, mutatingAbilities, "*"),
			}}
		},
	},
	{
		ID:          "guest-one-environment",
		Name:        "Guest in one environment",
		Description: "Read access to the apps and databases of one environment. Everything else is denied for changes.",
		Params:      []policyTemplateParam{{Name: templateParamEnvironment, Description: "Environment ID to grant read access to", Required: true, Kind: templateParamEnvironment}},
		build: func(p map[string]string) Document {
			return Document{Statement: []Statement{
				stmt(EffectAllow, []string{AbilityRead}, resourcePrefixEnvironment+p[templateParamEnvironment]),
				stmt(EffectDeny, mutatingAbilities, "*"),
			}}
		},
	},
	{
		ID:          "deployer-nonprod",
		Name:        "Deployer for non-production",
		Description: "Can deploy and change apps in dev, test, uat and preview environments. Production is denied.",
		build: func(map[string]string) Document {
			return Document{Statement: []Statement{
				stmt(EffectAllow, []string{AbilityRead, AbilityWrite, AbilityDeploy}, nonProductionKindResources...),
				stmt(EffectDeny, []string{AbilityWrite, AbilityWriteSensitive, AbilityDeploy}, productionKindResource),
			}}
		},
	},
	{
		ID:          "ai-operator-nonprod",
		Name:        "AI operator for non-production",
		Description: "For an AI agent token: can deploy and change apps in dev, test, uat and preview. Production and root are denied.",
		build: func(map[string]string) Document {
			return Document{Statement: []Statement{
				stmt(EffectAllow, []string{AbilityRead, AbilityWrite, AbilityDeploy}, nonProductionKindResources...),
				stmt(EffectDeny, mutatingAbilities, productionKindResource),
				stmt(EffectDeny, []string{AbilityRoot}, "*"),
			}}
		},
	},
	{
		ID:          "database-read-only",
		Name:        "Database read-only console",
		Description: "Can see one database and read its data in the console and explorer. Any change is denied.",
		Params:      []policyTemplateParam{{Name: templateParamDatabase, Description: "Database name", Required: true, Kind: templateParamDatabase}},
		build: func(p map[string]string) Document {
			res := resourcePrefixDatabase + p[templateParamDatabase]
			return Document{Statement: []Statement{
				stmt(EffectAllow, []string{AbilityRead, AbilityReadSensitive}, res),
				stmt(EffectDeny, mutatingAbilities, res),
			}}
		},
	},
	{
		ID:          "database-operator",
		Name:        "Database operator",
		Description: "Can read one database and run its day to day operations: backups, stop and start, public access. Users, restore and the write console need the owner.",
		Params:      []policyTemplateParam{{Name: templateParamDatabase, Description: "Database name", Required: true, Kind: templateParamDatabase}},
		build: func(p map[string]string) Document {
			res := resourcePrefixDatabase + p[templateParamDatabase]
			return Document{Statement: []Statement{
				stmt(EffectAllow, []string{AbilityRead, AbilityReadSensitive, AbilityWrite, AbilityWriteSensitive}, res),
				stmt(EffectDeny, []string{AbilityRoot}, res),
			}}
		},
	},
	{
		ID:          "database-owner",
		Name:        "Database owner",
		Description: "Full control of one database, including users, temporary credentials, restore and network rules. Nothing else on the platform.",
		Params:      []policyTemplateParam{{Name: templateParamDatabase, Description: "Database name", Required: true, Kind: templateParamDatabase}},
		build: func(p map[string]string) Document {
			res := resourcePrefixDatabase + p[templateParamDatabase]
			return Document{Statement: []Statement{
				stmt(EffectAllow, []string{AbilityRead, AbilityReadSensitive, AbilityWrite, AbilityWriteSensitive, AbilityRoot}, res),
			}}
		},
	},
	{
		ID:          "production-approver",
		Name:        "Production approver",
		Description: "Can see and approve deploys in production. Cannot change anything outside production.",
		build: func(map[string]string) Document {
			return Document{Statement: []Statement{
				stmt(EffectAllow, []string{AbilityRead, AbilityDeploy}, productionKindResource),
			}}
		},
	},
	{
		ID:          "app-operator",
		Name:        "Operator of one app",
		Description: "Can see, change and deploy one app. Nothing else is granted.",
		Params:      []policyTemplateParam{{Name: templateParamApp, Description: "App to operate", Required: true, Kind: templateParamApp}},
		build: func(p map[string]string) Document {
			return Document{Statement: []Statement{
				stmt(EffectAllow, []string{AbilityRead, AbilityWrite, AbilityDeploy}, resourcePrefixApp+p[templateParamApp]),
			}}
		},
	},
	{
		ID:          "project-deployer",
		Name:        "Deployer for one project",
		Description: "Can see, change and deploy everything in the environments of one project. Root is denied.",
		Params:      []policyTemplateParam{{Name: templateParamProject, Description: "Project whose environments to grant", Required: true, Kind: templateParamProject}},
		build: func(p map[string]string) Document {
			envs := []string{}
			for _, id := range strings.Split(p[templateParamProject+projectEnvironmentsSuffix], ",") {
				if id != "" {
					envs = append(envs, resourcePrefixEnvironment+id)
				}
			}
			if len(envs) == 0 {
				envs = []string{resourcePrefixEnvironment + "{" + templateParamProject + "}"}
			}
			return Document{Statement: []Statement{
				stmt(EffectAllow, []string{AbilityRead, AbilityWrite, AbilityDeploy}, envs...),
				stmt(EffectDeny, []string{AbilityRoot}, "*"),
			}}
		},
	},
}

func findPolicyTemplate(id string) (policyTemplate, bool) {
	i := slices.IndexFunc(policyTemplates, func(t policyTemplate) bool { return t.ID == id })
	if i < 0 {
		return policyTemplate{}, false
	}
	return policyTemplates[i], true
}

type policyTemplateResource struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Params      []policyTemplateParam `json:"params"`
	Document    Document              `json:"document"`
}

type policyTemplateListResponse struct {
	Version   int                      `json:"version"`
	Templates []policyTemplateResource `json:"templates"`
}

// placeholderParams renders a parameterized template for preview, showing
// each parameter as {name}.
func (t policyTemplate) placeholderParams() map[string]string {
	out := map[string]string{}
	for _, p := range t.Params {
		out[p.Name] = "{" + p.Name + "}"
	}
	return out
}

func (t policyTemplate) resource(params map[string]string) policyTemplateResource {
	params2 := t.Params
	if params2 == nil {
		params2 = []policyTemplateParam{}
	}
	return policyTemplateResource{ID: t.ID, Name: t.Name, Description: t.Description, Params: params2, Document: t.build(params)}
}

// handleListPolicyTemplates handles GET /api/v1/iam/policy-templates.
func (rt *Router) handleListPolicyTemplates(w http.ResponseWriter, _ *http.Request) {
	out := policyTemplateListResponse{Version: policyTemplatesVersion, Templates: make([]policyTemplateResource, 0, len(policyTemplates))}
	for _, t := range policyTemplates {
		out.Templates = append(out.Templates, t.resource(t.placeholderParams()))
	}
	writeJSON(w, http.StatusOK, out)
}

type applyTemplateRequest struct {
	Name   string            `json:"name"`
	Params map[string]string `json:"params"`
	Attach *struct {
		PrincipalType string `json:"principal_type"`
		PrincipalID   string `json:"principal_id"`
	} `json:"attach"`
}

type applyTemplateResponse struct {
	Policy   policyResource `json:"policy"`
	Attached bool           `json:"attached"`
}

var errTemplateParam = errors.New("invalid template parameter")

// validateTemplateParams checks every parameter and returns the params with
// derived values (a project's environment IDs) added for build.
func (rt *Router) validateTemplateParams(r *http.Request, t policyTemplate, params map[string]string) (map[string]string, error) {
	for _, p := range t.Params {
		v := params[p.Name]
		if v == "" && p.Required {
			return nil, fmt.Errorf("%w: %s is required", errTemplateParam, p.Name)
		}
		if v != "" && !templateParamPattern.MatchString(v) {
			return nil, fmt.Errorf("%w: %s must be an identifier", errTemplateParam, p.Name)
		}
	}
	for k := range params {
		if !slices.ContainsFunc(t.Params, func(p policyTemplateParam) bool { return p.Name == k }) {
			return nil, fmt.Errorf("%w: unknown parameter %q", errTemplateParam, k)
		}
	}
	out := maps.Clone(params)
	if out == nil {
		out = map[string]string{}
	}
	for _, p := range t.Params {
		if v := params[p.Name]; v != "" {
			if err := rt.checkTemplateParam(r, p, v, out); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

func (rt *Router) checkTemplateParam(r *http.Request, p policyTemplateParam, v string, out map[string]string) error {
	ctx := r.Context()
	missing := func(kind string) error { return fmt.Errorf("%w: %s %q does not exist", errTemplateParam, kind, v) }
	switch p.Kind {
	case templateParamEnvironment:
		if _, err := rt.environments.GetEnvironment(ctx, v); err != nil {
			if errors.Is(err, store.ErrEnvironmentNotFound) {
				return missing("environment")
			}
			return fmt.Errorf("look up environment: %w", err)
		}
	case templateParamApp:
		if _, err := rt.apps.GetDesiredService(ctx, v); err != nil {
			if errors.Is(err, store.ErrServiceNotFound) {
				return missing("app")
			}
			return fmt.Errorf("look up app: %w", err)
		}
	case templateParamDatabase:
		if _, err := rt.databases.GetDesiredDatabase(ctx, v); err != nil {
			if errors.Is(err, store.ErrDatabaseNotFound) {
				return missing("database")
			}
			return fmt.Errorf("look up database: %w", err)
		}
	case templateParamProject:
		envs, err := rt.environments.ListEnvironmentsByProject(ctx, v)
		if err != nil {
			return fmt.Errorf("look up project environments: %w", err)
		}
		if len(envs) == 0 {
			return fmt.Errorf("%w: project %q has no environments yet", errTemplateParam, v)
		}
		ids := make([]string, 0, len(envs))
		for _, e := range envs {
			ids = append(ids, e.ID)
		}
		out[p.Name+projectEnvironmentsSuffix] = strings.Join(ids, ",")
	}
	return nil
}

type renderTemplateRequest struct {
	Params map[string]string `json:"params"`
}

type renderTemplateResponse struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Document    Document `json:"document"`
}

// handleRenderPolicyTemplate handles POST /api/v1/iam/policy-templates/{id}/render:
// expand a template's parameters into a document without saving anything.
func (rt *Router) handleRenderPolicyTemplate(w http.ResponseWriter, r *http.Request) {
	t, ok := findPolicyTemplate(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "policy template not found")
		return
	}
	var req renderTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errInvalidPolicyRequestBody)
		return
	}
	params, err := rt.validateTemplateParams(r, t, req.Params)
	if errors.Is(err, errTemplateParam) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		rt.internalError(w, "api: render policy template: validate failed", err)
		return
	}
	writeJSON(w, http.StatusOK, renderTemplateResponse{Name: templatePolicyName(t, req.Params, ""), Description: t.Description, Document: t.build(params)})
}

func templatePolicyName(t policyTemplate, params map[string]string, override string) string {
	if override != "" {
		return override
	}
	name := t.ID
	for _, p := range t.Params {
		name += "-" + strings.ToLower(params[p.Name])
	}
	return name
}

// handleApplyPolicyTemplate handles POST /api/v1/iam/policy-templates/{id}/apply:
// create a policy from a template and optionally attach it to a user or token.
func (rt *Router) handleApplyPolicyTemplate(w http.ResponseWriter, r *http.Request) {
	t, ok := findPolicyTemplate(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "policy template not found")
		return
	}
	var req applyTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errInvalidPolicyRequestBody)
		return
	}
	if req.Attach != nil {
		if !slices.Contains(validPrincipalTypes, req.Attach.PrincipalType) || req.Attach.PrincipalID == "" {
			writeError(w, http.StatusBadRequest, "attach needs principal_type user or token and a principal_id")
			return
		}
	}
	params, err := rt.validateTemplateParams(r, t, req.Params)
	if err != nil {
		if errors.Is(err, errTemplateParam) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		rt.internalError(w, "api: apply policy template: validate failed", err)
		return
	}
	doc, err := json.Marshal(t.build(params))
	if err != nil {
		rt.internalError(w, "api: apply policy template: marshal failed", err)
		return
	}
	if _, err := ParseDocument(string(doc)); err != nil {
		rt.internalError(w, "api: apply policy template: rendered document invalid", err)
		return
	}
	if req.Attach != nil {
		ref := principalRef{Type: req.Attach.PrincipalType, ID: req.Attach.PrincipalID}
		if rt.enforceRootGuard(w, r, previewRequest{Document: doc, Attach: []principalRef{ref}}) {
			return
		}
	}
	id, err := store.NewPolicyID()
	if err != nil {
		rt.internalError(w, "api: apply policy template: generate id failed", err)
		return
	}
	now := time.Now()
	rec := store.Policy{ID: id, Name: templatePolicyName(t, req.Params, req.Name), Description: t.Description, Document: string(doc), CreatedAt: now, UpdatedAt: now}
	if err := rt.policies.SavePolicy(r.Context(), rec); err != nil {
		if errors.Is(err, store.ErrPolicyNameExists) {
			writeError(w, http.StatusConflict, "a policy with this name already exists")
			return
		}
		rt.internalError(w, "api: apply policy template: save failed", err)
		return
	}
	rt.recordPolicyVersion(r, rec)
	resp := applyTemplateResponse{Policy: toPolicyResource(rec)}
	if req.Attach != nil {
		attachID, err := store.NewPolicyAttachmentID()
		if err != nil {
			rt.internalError(w, "api: apply policy template: generate attachment id failed", err)
			return
		}
		if err := rt.policies.AttachPolicy(r.Context(), attachID, rec.ID, req.Attach.PrincipalType, req.Attach.PrincipalID); err != nil {
			rt.internalError(w, "api: apply policy template: attach failed", err)
			return
		}
		resp.Attached = true
	}
	writeJSON(w, http.StatusCreated, resp)
}
