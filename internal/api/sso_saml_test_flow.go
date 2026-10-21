package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	auditActionSAMLTest = "sso.saml.test_login"

	samlFailClockSkew   = "clock_skew"
	samlFailAudience    = "audience_mismatch"
	samlFailSignature   = "signature_invalid"
	samlFailRequest     = "request_mismatch"
	samlFailUnreadable  = "response_invalid"
	samlFailUnavailable = "saml_unavailable"
)

func samlTestKey(connectionID string) string { return "saml:" + connectionID }

// classifySAMLFailure names the setup mistake behind a refused response so the
// dashboard can say what to fix. The reason strings come from the verifier and
// never quote the document.
func classifySAMLFailure(err error) string {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "audience"):
		return samlFailAudience
	case strings.Contains(msg, "signature") || strings.Contains(msg, "not signed") || strings.Contains(msg, "digest"):
		return samlFailSignature
	case strings.Contains(msg, "expired") || strings.Contains(msg, "not yet valid") || strings.Contains(msg, "notbefore") ||
		strings.Contains(msg, "notonorafter") || strings.Contains(msg, "issueinstant"):
		return samlFailClockSkew
	case strings.Contains(msg, "inresponseto") || strings.Contains(msg, "destination") || strings.Contains(msg, "bearer confirmation"):
		return samlFailRequest
	}
	return samlFailUnreadable
}

// handleStartSAMLTest begins a test sign-in for the signed-in root user. The
// ACS stops after verifying, so no account, role or session is created.
func (rt *Router) handleStartSAMLTest(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.loadSAMLConnection(w, r)
	if !ok {
		return
	}
	userID, ok := rt.currentSessionUserID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "a test sign-in needs a dashboard session")
		return
	}
	if !samlUsable(c) {
		writeError(w, http.StatusBadRequest, "finish and enable this connection before testing it")
		return
	}
	if !requestIsHTTPS(r) {
		rt.putSAMLTest(userID, c.ID, ssoTestResult{Status: ssoTestStatusFailed, FailureCode: samlFailUnavailable})
		http.Redirect(w, r, samlTestReturn(c.ID), http.StatusFound)
		return
	}
	if err := rt.syncSAMLEndpoints(r.Context(), c); err != nil {
		rt.putSAMLTest(userID, c.ID, ssoTestResult{Status: ssoTestStatusFailed, FailureCode: samlFailUnavailable})
		http.Redirect(w, r, samlTestReturn(c.ID), http.StatusFound)
		return
	}
	rt.beginSAMLFlow(w, r, c, userID)
}

// handleSAMLTestResult returns, once, what the caller's last test sign-in saw.
func (rt *Router) handleSAMLTestResult(w http.ResponseWriter, r *http.Request) {
	userID, ok := rt.currentSessionUserID(r)
	if !ok || rt.ssoTests == nil {
		writeJSON(w, http.StatusOK, ssoTestResult{Status: ssoTestStatusNone})
		return
	}
	writeJSON(w, http.StatusOK, rt.ssoTests.take(userID, samlTestKey(r.PathValue("id"))))
}

func samlTestReturn(connectionID string) string {
	return "/settings/sso?sso_test=" + url.QueryEscape(samlTestKey(connectionID))
}

func (rt *Router) putSAMLTest(userID, connectionID string, res ssoTestResult) {
	if rt.ssoTests != nil {
		rt.ssoTests.put(ssoTestFlow{userID: userID, provider: samlTestKey(connectionID)}, res)
	}
}

// finishSAMLTest verifies the response and stores what it carried and what a
// real sign-in would do with it.
func (rt *Router) finishSAMLTest(w http.ResponseWriter, r *http.Request, c store.SSOConnection, flow samlFlow, samlResponse string) {
	res := ssoTestResult{Status: ssoTestStatusFailed}
	ident, err := rt.authEngine.VerifySAMLResponse(r.Context(), c.LibraryID, samlResponse, flow.requestID)
	if err != nil {
		res.FailureCode = classifySAMLFailure(err)
		rt.logger.Warn("api: saml test sign-in refused", slog.String("connection_id", c.ID), slog.String("code", res.FailureCode), slog.String("detail", err.Error()))
	} else if ev, err := rt.evaluateSAMLSignIn(r.Context(), c, ident); err != nil {
		rt.logger.Error("api: saml test evaluation failed", slog.String("connection_id", c.ID), slog.String("error", err.Error()))
		res.FailureCode = samlErrInternal
	} else {
		res = ssoTestResult{Status: ssoTestStatusOK, Evaluation: &ev}
	}
	rt.putSAMLTest(flow.testUser, c.ID, res)
	rt.auditSignIn(r.Context(), r, signInActor{kind: auditActorTypeSession, id: flow.testUser}, auditActionSAMLTest,
		samlPathPrefix+c.ID+"/acs?status="+res.Status, http.StatusFound)
	http.Redirect(w, r, samlTestReturn(c.ID), http.StatusFound)
}

// evaluateSAMLSignIn mirrors samlPlanSignIn on the facts of an assertion and
// changes nothing.
func (rt *Router) evaluateSAMLSignIn(ctx context.Context, c store.SSOConnection, ident authengine.SAMLAssertion) (ssoEvaluation, error) {
	ev := ssoEvaluation{
		Provider: c.ID, Email: strings.ToLower(ident.Email), EmailVerified: true, GroupsClaim: "",
		GroupsFound: len(ident.Groups) > 0, Groups: nonNilStrings(ident.Groups), GroupLikeClaims: []string{},
		SyncRoles: c.SyncRoles, Allowed: true, Issues: []ssoIssue{},
	}
	refuse := func(code string) {
		if ev.Allowed {
			ev.Allowed, ev.RefusalCode = false, code
		}
	}
	if b, err := rt.samlBinding(ctx, c); err == nil {
		ev.GroupsClaim = b.GroupsAttribute
	}
	mappings, err := rt.sso.ListSSOGroupMappings(ctx)
	if err != nil {
		return ssoEvaluation{}, err
	}
	role, source := c.DefaultRoleID, ssoRoleSourceDefault
	if id, ok := resolveSSORole(ident.Groups, mappings); ok {
		role, source = id, ssoRoleSourceMapping
	}
	ev.RoleID, ev.RoleSource = role, source
	if role == "" {
		refuse(samlErrNoRole)
	} else if err := rt.ssoGrantableRole(ctx, role); err != nil {
		if !errors.Is(err, errSSORoleGrantsRoot) && !errors.Is(err, store.ErrRoleNotFound) {
			return ssoEvaluation{}, err
		}
		ev.Issues = append(ev.Issues, ssoIssue{Code: ssoIssueRoleIsRoot, Fix: ssoFix{Kind: ssoFixManual}})
		refuse(samlErrNoRole)
	}
	if ev.Email == "" {
		ev.Issues = append(ev.Issues, ssoIssue{Code: ssoIssueNoEmail, Fix: ssoFix{Kind: ssoFixManual}})
		refuse(samlErrNoEmail)
	} else if !emailDomainAllowed(ev.Email, c.AllowedDomains) {
		domain := ev.Email[strings.LastIndexByte(ev.Email, '@')+1:]
		ev.Issues = append(ev.Issues, ssoIssue{Code: ssoIssueDomain, Fix: ssoFix{Kind: ssoFixAuto, Action: "add_domain", Value: domain}})
		refuse(samlErrDomain)
	}
	if err := rt.evaluateSAMLAccount(ctx, c, ident, &ev, refuse); err != nil {
		return ssoEvaluation{}, err
	}
	if !ev.GroupsFound {
		ev.Issues = append(ev.Issues, ssoIssue{Code: ssoIssueNoGroups, Fix: ssoFix{Kind: ssoFixManual}})
	}
	return ev, nil
}

func (rt *Router) evaluateSAMLAccount(ctx context.Context, c store.SSOConnection, ident authengine.SAMLAssertion, ev *ssoEvaluation, refuse func(string)) error {
	ev.AccountState = ssoAccountNotAllowed
	linked, err := rt.authEngine.SAMLIdentityUser(ctx, c.LibraryID, ident.NameID)
	if err != nil {
		return err
	}
	if linked != "" {
		ev.AccountState = ssoAccountLinked
		return nil
	}
	if ev.Email != "" {
		_, err := rt.auth.GetUserByEmail(ctx, ev.Email)
		switch {
		case err == nil:
			ev.AccountState = ssoAccountByEmail
			return nil
		case !errors.Is(err, store.ErrUserNotFound):
			return err
		}
	}
	if c.JITProvisioning {
		ev.AccountState = ssoAccountNew
		return nil
	}
	ev.Issues = append(ev.Issues, ssoIssue{Code: ssoIssueNotProvisioned, Fix: ssoFix{Kind: ssoFixAuto, Action: "enable_jit"}})
	refuse(samlErrNotProvisioned)
	return nil
}
