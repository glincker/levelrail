package ingress

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
)

// Headers of the route check a verifier sends through a fronting proxy.
const (
	RouteCheckHeader      = "X-Ingress-Route-Check"
	RouteCheckReplyHeader = "X-Ingress-Route-Reply"
)

var routeCheckTokenRE = regexp.MustCompile(`^[0-9a-f]{32}$`)

// RouteCheck proves a request reached this ingress: a request carrying Token
// in RouteCheckHeader gets 204 with Reply, which no app behind it can know.
type RouteCheck struct {
	Token string
	Reply string
}

// NewRouteCheck returns a random token pair, valid for this process.
func NewRouteCheck() (RouteCheck, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return RouteCheck{}, err
	}
	return RouteCheck{Token: hex.EncodeToString(b[:16]), Reply: hex.EncodeToString(b[16:])}, nil
}

// Valid reports whether both values are well formed and distinct.
func (c RouteCheck) Valid() bool {
	return routeCheckTokenRE.MatchString(c.Token) && routeCheckTokenRE.MatchString(c.Reply) && c.Token != c.Reply
}

func (c RouteCheck) route() (Route, error) {
	if !c.Valid() {
		return Route{}, errors.New("ingress: route check tokens are malformed")
	}
	return Route{
		Match: []Matcher{{Header: map[string][]string{RouteCheckHeader: {c.Token}}}},
		Handle: []any{StaticResponseHandler{
			Handler:    "static_response",
			StatusCode: 204,
			Headers:    map[string][]string{RouteCheckReplyHeader: {c.Reply}, "Cache-Control": {"no-store"}},
		}},
	}, nil
}
