package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const (
	issueError   = "error"
	issueWarning = "warning"
)

// fieldIssue points at the document field a problem belongs to, as a path
// such as Statement[1].Action[0], so the builder can mark that exact control.
type fieldIssue struct {
	Path     string `json:"path"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

// knownStatementKeys are the only statement keys the evaluator reads.
var knownStatementKeys = []string{"Effect", "Action", "Resource"}

type rawStatement map[string]json.RawMessage

func fieldPath(i int, field string, j int) string {
	if j < 0 {
		return fmt.Sprintf("Statement[%d].%s", i, field)
	}
	return fmt.Sprintf("Statement[%d].%s[%d]", i, field, j)
}

// validateDocumentFields reports every problem in a document at once, with
// field paths. The error set is exactly ParseDocument's: a document with no
// error issues parses.
func validateDocumentFields(raw string, inv *iamInventory) []fieldIssue {
	var top struct {
		Statement []rawStatement `json:"Statement"`
	}
	if err := json.Unmarshal([]byte(raw), &top); err != nil {
		return []fieldIssue{{Path: "", Message: "document is not valid JSON: " + err.Error(), Severity: issueError}}
	}
	if len(top.Statement) == 0 {
		return []fieldIssue{{Path: "Statement", Message: errDocumentNoStatements.Error(), Severity: issueError}}
	}
	var issues []fieldIssue
	for i, rs := range top.Statement {
		issues = append(issues, validateRawStatement(i, rs, inv)...)
	}
	return issues
}

func validateRawStatement(i int, rs rawStatement, inv *iamInventory) []fieldIssue {
	var issues []fieldIssue
	add := func(path, msg, sev string) {
		issues = append(issues, fieldIssue{Path: path, Message: msg, Severity: sev})
	}

	var s Statement
	b, _ := json.Marshal(rs)
	if err := json.Unmarshal(b, &s); err != nil {
		add(fmt.Sprintf("Statement[%d]", i), "statement has the wrong shape: "+err.Error(), issueError)
		return issues
	}
	if s.Effect != EffectAllow && s.Effect != EffectDeny {
		add(fieldPath(i, "Effect", -1), errStatementBadEffect.Error(), issueError)
	}
	if len(s.Action) == 0 {
		add(fieldPath(i, "Action", -1), errStatementNoAction.Error(), issueError)
	}
	for j, a := range s.Action {
		if a != "*" && !isKnownAbility(a) {
			add(fieldPath(i, "Action", j), (&unknownActionError{action: a}).Error(), issueError)
		}
	}
	if len(s.Resource) == 0 {
		add(fieldPath(i, "Resource", -1), errStatementNoResource.Error(), issueError)
	}
	for j, r := range s.Resource {
		if strings.TrimSpace(r) == "" {
			add(fieldPath(i, "Resource", j), errStatementNoResource.Error(), issueError)
			continue
		}
		if err := validateEnvironmentResource(r); err != nil {
			add(fieldPath(i, "Resource", j), err.Error(), issueError)
			continue
		}
		if inv != nil && !strings.Contains(r, "*") && !inv.resourceExists(r) {
			add(fieldPath(i, "Resource", j), "nothing matches "+r+" today", issueWarning)
		}
	}
	for k := range rs {
		if !containsString(knownStatementKeys, k) {
			add(fmt.Sprintf("Statement[%d].%s", i, k), k+" is not read by the evaluator, so this statement applies without it", issueWarning)
		}
	}
	return issues
}

func containsString(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func hasErrorIssue(issues []fieldIssue) bool {
	for _, i := range issues {
		if i.Severity == issueError {
			return true
		}
	}
	return false
}

type validateRequest struct {
	Document json.RawMessage `json:"document"`
}

type validateResponse struct {
	Valid    bool         `json:"valid"`
	Issues   []fieldIssue `json:"issues"`
	Findings []finding    `json:"findings"`
}

var errNoDocument = errors.New("document is required")

// handleValidatePolicy handles POST /api/v1/iam/policies/validate: field level
// validation plus the analyzer's statement checks. It persists nothing.
func (rt *Router) handleValidatePolicy(w http.ResponseWriter, r *http.Request) {
	var req validateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Document) == 0 {
		writeError(w, http.StatusBadRequest, errNoDocument.Error())
		return
	}
	inv, err := rt.loadIAMInventory(r.Context())
	if err != nil {
		rt.internalError(w, "api: validate policy: inventory failed", err)
		return
	}
	issues := validateDocumentFields(string(req.Document), inv)
	if issues == nil {
		issues = []fieldIssue{}
	}
	resp := validateResponse{Valid: !hasErrorIssue(issues), Issues: issues, Findings: []finding{}}
	if resp.Valid {
		if doc, perr := ParseDocument(string(req.Document)); perr == nil {
			resp.Findings = analyzeDocument("", "", doc, inv)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}
