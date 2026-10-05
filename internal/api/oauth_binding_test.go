package api

import (
	"net/http"
	"net/http/httptest"
	"sync"
)

var oauthTestBindings sync.Map

func oauthCallbackRequest(state string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/google/callback?state="+state+"&code=abc", nil)
	if c, ok := oauthTestBindings.Load(state); ok {
		if cookie, isCookie := c.(*http.Cookie); isCookie {
			req.AddCookie(cookie)
		}
	}
	return req
}

func mismatchedBinding(c *http.Cookie) *http.Cookie {
	return &http.Cookie{Name: c.Name, Value: "attacker"} //nolint:gosec // test fixture
}
