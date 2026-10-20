package api

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
)

const (
	ssoTestCookie      = "sso_test"
	ssoTestTTL         = 10 * time.Minute
	ssoTestMaxInFlight = 1000

	auditActionOIDCTest = "sso.oidc.test_login"

	ssoTestStatusNone   = "none"
	ssoTestStatusOK     = "ok"
	ssoTestStatusFailed = "failed"
)

type ssoTestFlow struct {
	userID, provider string
	expires          time.Time
}

// ssoTestResult is what a test login saw. FailureCode is the short sign-in
// code when the provider exchange itself failed.
type ssoTestResult struct {
	Status      string         `json:"status"`
	FailureCode string         `json:"failure_code,omitempty"`
	Evaluation  *ssoEvaluation `json:"evaluation,omitempty"`
	expires     time.Time
}

// ssoTestStore holds test logins in memory: a pending flow is bound to the
// browser by an unguessable cookie, and a result is kept for the root user who
// started it until read or expired. Nothing here is persisted.
type ssoTestStore struct {
	mu      sync.Mutex
	flows   map[string]ssoTestFlow
	results map[string]ssoTestResult
}

func newSSOTestStore() *ssoTestStore {
	return &ssoTestStore{flows: map[string]ssoTestFlow{}, results: map[string]ssoTestResult{}}
}

func ssoTestKey(userID, provider string) string { return userID + "|" + provider }

func (s *ssoTestStore) begin(userID, provider string, now time.Time) (string, bool) {
	secret, err := randomToken()
	if err != nil {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.flows) >= ssoTestMaxInFlight {
		for k, f := range s.flows {
			if now.After(f.expires) {
				delete(s.flows, k)
			}
		}
		if len(s.flows) >= ssoTestMaxInFlight {
			return "", false
		}
	}
	s.flows[hashToken(secret)] = ssoTestFlow{userID: userID, provider: provider, expires: now.Add(ssoTestTTL)}
	return secret, true
}

// consumeFlow reports whether the request carries a live test cookie for
// provider, spending it. A nil store never matches.
func (s *ssoTestStore) consumeFlow(r *http.Request, provider string) (ssoTestFlow, bool) {
	if s == nil {
		return ssoTestFlow{}, false
	}
	c, err := r.Cookie(ssoTestCookie)
	if err != nil || c.Value == "" {
		return ssoTestFlow{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := hashToken(c.Value)
	f, ok := s.flows[key]
	delete(s.flows, key)
	if !ok || f.provider != provider || time.Now().After(f.expires) {
		return ssoTestFlow{}, false
	}
	return f, true
}

func (s *ssoTestStore) put(f ssoTestFlow, res ssoTestResult) {
	res.expires = time.Now().Add(ssoTestTTL)
	s.mu.Lock()
	s.results[ssoTestKey(f.userID, f.provider)] = res
	s.mu.Unlock()
}

func (s *ssoTestStore) take(userID, provider string) ssoTestResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := ssoTestKey(userID, provider)
	res, ok := s.results[key]
	delete(s.results, key)
	if !ok || time.Now().After(res.expires) {
		return ssoTestResult{Status: ssoTestStatusNone}
	}
	return res
}

func setSSOTestCookie(w http.ResponseWriter, r *http.Request, provider, value string) {
	c := &http.Cookie{ //nolint:gosec // Secure follows the client transport, see requestIsHTTPS
		Name: ssoTestCookie, Value: value, Path: authengine.OAuthCallbackPath(provider),
		HttpOnly: true, Secure: requestIsHTTPS(r), SameSite: http.SameSiteLaxMode, MaxAge: int(ssoTestTTL / time.Second)}
	if value == "" {
		c.MaxAge = -1
	}
	http.SetCookie(w, c) // NOSONAR: Secure follows the client transport, see requestIsHTTPS
}

// handleStartOAuthSSOTest begins a test login for the signed-in root user. The
// callback stops after the provider answers, so no account, role or session
// is created.
func (rt *Router) handleStartOAuthSSOTest(w http.ResponseWriter, r *http.Request) {
	s, ok := rt.loadOAuthSettings(w, r)
	if !ok {
		return
	}
	if !rt.authLibOAuthActive() || rt.ssoTests == nil {
		writeError(w, http.StatusNotImplemented, "oauth sign-in is not configured")
		return
	}
	userID, ok := rt.currentSessionUserID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "a test login needs a dashboard session")
		return
	}
	if !s.Enabled {
		writeError(w, http.StatusBadRequest, "enable this provider before testing it")
		return
	}
	secret, ok := rt.ssoTests.begin(userID, s.Provider, time.Now())
	if !ok {
		writeError(w, http.StatusTooManyRequests, "too many test logins in flight")
		return
	}
	setSSOTestCookie(w, r, s.Provider, secret)
	rt.authLibOAuthStart(w, r)
}

// handleOAuthSSOTestResult returns, once, what the caller's last test login saw.
func (rt *Router) handleOAuthSSOTestResult(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if !isValidOAuthProvider(provider) || rt.ssoTests == nil {
		writeError(w, http.StatusNotFound, "unknown oauth provider")
		return
	}
	userID, ok := rt.currentSessionUserID(r)
	if !ok {
		writeJSON(w, http.StatusOK, ssoTestResult{Status: ssoTestStatusNone})
		return
	}
	writeJSON(w, http.StatusOK, rt.ssoTests.take(userID, provider))
}

// finishSSOTest turns the dry-run callback into a stored result and sends the
// browser back to the settings page.
func (rt *Router) finishSSOTest(w http.ResponseWriter, r *http.Request, provider string, f ssoTestFlow, out authengine.OAuthOutcome) {
	setSSOTestCookie(w, r, provider, "")
	res := ssoTestResult{Status: ssoTestStatusFailed, FailureCode: out.ErrorCode}
	if out.ErrorCode == authengine.OAuthErrDryRunComplete {
		policy, err := rt.oauthPolicy(r.Context(), provider)
		if err == nil {
			var ev ssoEvaluation
			if ev, err = rt.evaluateOAuthSignIn(r.Context(), provider, policy, out.Facts); err == nil {
				res = ssoTestResult{Status: ssoTestStatusOK, Evaluation: &ev}
			}
		}
		if err != nil {
			rt.logger.Error("api: sso test evaluation failed", slog.String("provider", provider), slog.String("error", err.Error()))
			res.FailureCode = samlErrInternal
		}
	}
	rt.ssoTests.put(f, res)
	rt.auditSignIn(r.Context(), r, signInActor{kind: auditActorTypeSession, id: f.userID}, auditActionOIDCTest,
		oauthCallbackAuditPath+provider+"/callback?status="+res.Status, http.StatusFound)
	http.Redirect(w, r, "/settings/sso?sso_test="+url.QueryEscape(provider), http.StatusFound)
}

// evaluationFacts builds the facts of a what-if evaluation from a request.
func evaluationFacts(provider string, req oauthSSOEvaluateRequest) authengine.OAuthIdentityFacts {
	return authengine.OAuthIdentityFacts{
		Provider: provider, Email: strings.ToLower(strings.TrimSpace(req.Email)), EmailVerified: req.EmailVerified,
		Claims: authengine.IDTokenClaims{Groups: req.Groups, GroupsFound: req.Groups != nil},
	}
}
