package authengine

import (
	"context"
	"net/http"
	"strings"
	"sync"
)

// Short error codes the dashboard login page already understands.
const (
	OAuthErrInvalidProvider  = "invalid_provider"
	OAuthErrInvalidRequest   = "invalid_request"
	OAuthErrInvalidState     = "invalid_state"
	OAuthErrProviderDenied   = "provider_denied"
	OAuthErrProviderDisabled = "provider_disabled"
	OAuthErrExchangeFailed   = "exchange_failed"
	OAuthErrUserInfoFailed   = "userinfo_failed"
	OAuthErrEmailInUse       = "email_in_use"
	OAuthErrDomainNotAllowed = "domain_not_allowed"
	OAuthErrSigninFailed     = "signin_failed"
	OAuthErrInternal         = "internal_error"

	sessionCookieName = cookieName
)

type flowKey struct{}

// oauthFlow records what the provider adapter saw during one library request,
// because the library only answers a generic failure.
type oauthFlow struct {
	mu             sync.Mutex
	code           string
	exchangeCalled bool
	provider       string
	providerUserID string
}

func flowFrom(ctx context.Context) *oauthFlow {
	f, _ := ctx.Value(flowKey{}).(*oauthFlow)
	return f
}

func (f *oauthFlow) fail(code string) {
	if f == nil {
		return
	}
	f.mu.Lock()
	if f.code == "" {
		f.code = code
	}
	f.mu.Unlock()
}

func (f *oauthFlow) markExchange() {
	if f == nil {
		return
	}
	f.mu.Lock()
	f.exchangeCalled = true
	f.mu.Unlock()
}

func (f *oauthFlow) identify(provider, providerUserID string) {
	if f == nil {
		return
	}
	f.mu.Lock()
	f.provider, f.providerUserID = provider, providerUserID
	f.mu.Unlock()
}

// OAuthOutcome is the result of driving one library OAuth request.
type OAuthOutcome struct {
	Status   int
	Location string
	// Cookies are the flow cookies to forward to the browser (never the library session).
	Cookies []*http.Cookie
	// SessionCookie is the library session issued on success, nil otherwise.
	SessionCookie *http.Cookie
	// ErrorCode is the short legacy code for a failed callback.
	ErrorCode string
	// Provider and ProviderUserID identify the external account on success.
	Provider       string
	ProviderUserID string
}

type captureWriter struct {
	header http.Header
	status int
	body   strings.Builder
}

func newCaptureWriter() *captureWriter { return &captureWriter{header: http.Header{}} }

func (c *captureWriter) Header() http.Header { return c.header }
func (c *captureWriter) WriteHeader(code int) {
	if c.status == 0 {
		c.status = code
	}
}
func (c *captureWriter) Write(b []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	const maxBody = 1 << 10
	if c.body.Len() < maxBody {
		c.body.Write(b)
	}
	return len(b), nil
}

func (e *Engine) drive(r *http.Request, route, provider string) (OAuthOutcome, *oauthFlow) {
	flow := &oauthFlow{}
	r2 := r.Clone(context.WithValue(r.Context(), flowKey{}, flow))
	r2.URL.Path = e.prefix + "/providers/" + provider + "/" + route
	r2.URL.RawPath = ""
	r2.RequestURI = r2.URL.RequestURI()
	cw := newCaptureWriter()
	e.Handler().ServeHTTP(cw, r2)

	out := OAuthOutcome{Status: cw.status, Location: cw.header.Get("Location")}
	resp := http.Response{Header: cw.header}
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookieName {
			if c.MaxAge >= 0 && c.Value != "" {
				out.SessionCookie = c
			}
			continue
		}
		out.Cookies = append(out.Cookies, c)
	}
	return out, flow
}

// OAuthStart runs the library start step for provider. A 404 status means the
// provider is unknown or disabled, a 503 means its settings could not be resolved.
func (e *Engine) OAuthStart(r *http.Request, provider string) OAuthOutcome {
	out, _ := e.drive(r, "start", provider)
	return out
}

// OAuthCallback runs the library callback step and classifies any failure
// into a legacy short code.
func (e *Engine) OAuthCallback(r *http.Request, provider string) OAuthOutcome {
	q := r.URL.Query()
	if q.Get("error") != "" {
		return OAuthOutcome{Status: http.StatusBadRequest, ErrorCode: OAuthErrProviderDenied}
	}
	if q.Get("state") == "" || q.Get("code") == "" {
		return OAuthOutcome{Status: http.StatusBadRequest, ErrorCode: OAuthErrInvalidRequest}
	}
	out, flow := e.drive(r, "callback", provider)
	flow.mu.Lock()
	defer flow.mu.Unlock()
	out.Provider, out.ProviderUserID = flow.provider, flow.providerUserID
	if out.Status == http.StatusFound && out.SessionCookie != nil {
		if flow.provider == "" || flow.providerUserID == "" {
			out.ErrorCode = OAuthErrInternal
		}
		return out
	}
	out.SessionCookie = nil
	out.ErrorCode = classifyCallback(out.Status, flow)
	return out
}

func classifyCallback(status int, f *oauthFlow) string {
	switch {
	case f.code != "":
		return f.code
	case status == http.StatusNotFound:
		return OAuthErrProviderDisabled
	case status == http.StatusServiceUnavailable || status >= http.StatusInternalServerError:
		return OAuthErrInternal
	case status != http.StatusBadRequest:
		return OAuthErrSigninFailed
	case !f.exchangeCalled:
		return OAuthErrInvalidState
	default:
		return OAuthErrSigninFailed
	}
}
