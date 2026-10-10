package api

import (
	"encoding/json"
	"errors"
	"fmt"
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
	templateParamDatabase    = "database"
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
		Params:      []policyTemplateParam{{Name: templateParamEnvironment, Description: "Environment ID to grant read access to", Required: true}},
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
		Params:      []policyTemplateParam{{Name: templateParamDatabase, Description: "Database name", Required: true}},
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
		Params:      []policyTemplateParam{{Name: templateParamDatabase, Description: "Database name", Required: true}},
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
		Params:      []policyTemplateParam{{Name: templateParamDatabase, Description: "Database name", Required: true}},
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

func (rt *Router) validateTemplateParams(r *http.Request, t policyTemplate, params map[string]string) error {
	for _, p := range t.Params {
		v := params[p.Name]
		if v == "" && p.Required {
			return fmt.Errorf("%w: %s is required", errTemplateParam, p.Name)
		}
		if v != "" && !templateParamPattern.MatchString(v) {
			return fmt.Errorf("%w: %s must be an identifier", errTemplateParam, p.Name)
		}
	}
	for k := range params {
		if !slices.ContainsFunc(t.Params, func(p policyTemplateParam) bool { return p.Name == k }) {
			return fmt.Errorf("%w: unknown parameter %q", errTemplateParam, k)
		}
	}
	if name, ok := params[templateParamDatabase]; ok && name != "" {
		if _, err := rt.databases.GetDesiredDatabase(r.Context(), name); err != nil {
			if errors.Is(err, store.ErrDatabaseNotFound) {
				return fmt.Errorf("%w: database %q does not exist", errTemplateParam, name)
			}
			return fmt.Errorf("look up database: %w", err)
		}
	}
	if env, ok := params[templateParamEnvironment]; ok && t.ID == "guest-one-environment" {
		if _, err := rt.environments.GetEnvironment(r.Context(), env); err != nil {
			if errors.Is(err, store.ErrEnvironmentNotFound) {
				return fmt.Errorf("%w: environment %q does not exist", errTemplateParam, env)
			}
			return fmt.Errorf("look up environment: %w", err)
		}
	}
	return nil
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
	if err := rt.validateTemplateParams(r, t, req.Params); err != nil {
		if errors.Is(err, errTemplateParam) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		rt.internalError(w, "api: apply policy template: validate failed", err)
		return
	}
	doc, err := json.Marshal(t.build(req.Params))
	if err != nil {
		rt.internalError(w, "api: apply policy template: marshal failed", err)
		return
	}
	if _, err := ParseDocument(string(doc)); err != nil {
		rt.internalError(w, "api: apply policy template: rendered document invalid", err)
		return
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
