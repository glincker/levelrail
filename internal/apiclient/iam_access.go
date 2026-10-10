package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// IAMStatementRef mirrors internal/api's statementRef.
type IAMStatementRef struct {
	PolicyID       string   `json:"policy_id"`
	PolicyName     string   `json:"policy_name"`
	StatementIndex int      `json:"statement_index"`
	Effect         string   `json:"effect"`
	Action         []string `json:"action"`
	Resource       []string `json:"resource"`
}

// IAMSimulation mirrors internal/api's simulation.
type IAMSimulation struct {
	PrincipalType        string            `json:"principal_type"`
	PrincipalID          string            `json:"principal_id"`
	PrincipalName        string            `json:"principal_name"`
	Action               string            `json:"action"`
	Resource             string            `json:"resource"`
	Allowed              bool              `json:"allowed"`
	DecidedBy            string            `json:"decided_by"`
	DecidingStatement    *IAMStatementRef  `json:"deciding_statement"`
	MatchedStatements    []IAMStatementRef `json:"matched_statements"`
	BaseAbilityGrants    bool              `json:"base_ability_grants"`
	EnvironmentResources []string          `json:"environment_resources"`
}

// IAMEffectiveAbility mirrors internal/api's effectiveAbility.
type IAMEffectiveAbility struct {
	Ability         string   `json:"ability"`
	Risk            string   `json:"risk"`
	Flat            bool     `json:"flat"`
	All             bool     `json:"all"`
	Allowed         int      `json:"allowed"`
	Total           int      `json:"total"`
	Apps            []string `json:"apps"`
	Databases       []string `json:"databases"`
	GrantedByPolicy int      `json:"granted_by_policy"`
	DeniedByPolicy  int      `json:"denied_by_policy"`
}

// IAMPolicyBrief is a policy id and name.
type IAMPolicyBrief struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// IAMPrincipal mirrors internal/api's iamPrincipal.
type IAMPrincipal struct {
	PrincipalType string   `json:"principal_type"`
	PrincipalID   string   `json:"principal_id"`
	Name          string   `json:"name"`
	Detail        string   `json:"detail,omitempty"`
	Abilities     []string `json:"abilities"`
	Active        bool     `json:"active"`
	PolicyIDs     []string `json:"policy_ids"`
}

// IAMEffective mirrors internal/api's effectiveResponse.
type IAMEffective struct {
	Principal  IAMPrincipal          `json:"principal"`
	Policies   []IAMPolicyBrief      `json:"policies"`
	Restricted bool                  `json:"restricted"`
	Abilities  []IAMEffectiveAbility `json:"abilities"`
}

// IAMFinding mirrors internal/api's finding.
type IAMFinding struct {
	Kind           string `json:"kind"`
	Severity       string `json:"severity"`
	PolicyID       string `json:"policy_id,omitempty"`
	PolicyName     string `json:"policy_name,omitempty"`
	StatementIndex *int   `json:"statement_index,omitempty"`
	PrincipalType  string `json:"principal_type,omitempty"`
	PrincipalID    string `json:"principal_id,omitempty"`
	Message        string `json:"message"`
	Fix            string `json:"fix"`
}

// IAMAnalysis mirrors internal/api's analysisResponse.
type IAMAnalysis struct {
	Score    int            `json:"score"`
	Counts   map[string]int `json:"counts"`
	Findings []IAMFinding   `json:"findings"`
}

// SimulateIAM calls GET /api/v1/iam/simulate.
func (c *Client) SimulateIAM(ctx context.Context, principalType, principalID, action, resource string) (IAMSimulation, error) {
	q := url.Values{"principal_type": {principalType}, "principal_id": {principalID}, "action": {action}, "resource": {resource}}
	var out IAMSimulation
	err := c.do(ctx, http.MethodGet, "/api/v1/iam/simulate?"+q.Encode(), nil, &out)
	return out, err
}

// IAMEffectivePermissions calls GET /api/v1/iam/principals/{type}/{id}/effective.
func (c *Client) IAMEffectivePermissions(ctx context.Context, principalType, principalID string) (IAMEffective, error) {
	var out IAMEffective
	err := c.do(ctx, http.MethodGet, "/api/v1/iam/principals/"+PathEscape(principalType)+"/"+PathEscape(principalID)+"/effective", nil, &out)
	return out, err
}

// AnalyzeIAM calls GET /api/v1/iam/analyze.
func (c *Client) AnalyzeIAM(ctx context.Context) (IAMAnalysis, error) {
	var out IAMAnalysis
	err := c.do(ctx, http.MethodGet, "/api/v1/iam/analyze", nil, &out)
	return out, err
}
