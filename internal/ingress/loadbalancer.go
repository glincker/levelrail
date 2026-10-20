package ingress

// Caddy reverse_proxy upstream pool shapes used by the load balancer.
// The product-level model lives in internal/loadbalancer; this file only
// knows how to render it as Caddy JSON.

// Selection policy names accepted by LBRoute.Policy.
const (
	LBPolicyRoundRobin = "round_robin"
	LBPolicyLeastConn  = "least_conn"
	LBPolicyIPHash     = "ip_hash"
	LBPolicyURIHash    = "uri_hash"
	LBPolicyCookie     = "cookie"
	LBPolicyWeighted   = "weighted_round_robin"
)

// SelectionPolicy is Caddy's load_balancing.selection_policy.
type SelectionPolicy struct {
	Policy   string           `json:"policy"`
	Weights  []int            `json:"weights,omitempty"`
	Name     string           `json:"name,omitempty"`
	MaxAge   string           `json:"max_age,omitempty"`
	Fallback *SelectionPolicy `json:"fallback,omitempty"`
}

// LoadBalancing is Caddy's reverse_proxy load_balancing block.
type LoadBalancing struct {
	SelectionPolicy *SelectionPolicy `json:"selection_policy,omitempty"`
	Retries         int              `json:"retries,omitempty"`
	TryDuration     string           `json:"try_duration,omitempty"`
	TryInterval     string           `json:"try_interval,omitempty"`
}

// ActiveHealthChecks is Caddy's health_checks.active block.
type ActiveHealthChecks struct {
	URI          string `json:"uri,omitempty"`
	Interval     string `json:"interval,omitempty"`
	Timeout      string `json:"timeout,omitempty"`
	Passes       int    `json:"passes,omitempty"`
	Fails        int    `json:"fails,omitempty"`
	ExpectStatus int    `json:"expect_status,omitempty"`
}

// PassiveHealthChecks is Caddy's health_checks.passive block.
type PassiveHealthChecks struct {
	FailDuration    string `json:"fail_duration,omitempty"`
	MaxFails        int    `json:"max_fails,omitempty"`
	UnhealthyStatus []int  `json:"unhealthy_status,omitempty"`
}

// HealthChecks is Caddy's reverse_proxy health_checks block.
type HealthChecks struct {
	Active  *ActiveHealthChecks  `json:"active,omitempty"`
	Passive *PassiveHealthChecks `json:"passive,omitempty"`
}

// UpstreamTLSConfig is the transport-level tls block toward upstreams.
type UpstreamTLSConfig struct {
	InsecureSkipVerify bool   `json:"insecure_skip_verify,omitempty"`
	ServerName         string `json:"server_name,omitempty"`
}

// HTTPTransport is Caddy's reverse_proxy http transport.
type HTTPTransport struct {
	Protocol              string             `json:"protocol"`
	TLS                   *UpstreamTLSConfig `json:"tls,omitempty"`
	DialTimeout           string             `json:"dial_timeout,omitempty"`
	ResponseHeaderTimeout string             `json:"response_header_timeout,omitempty"`
}

// LBRoute is one route's upstream pool and balancing settings. A
// ProxyRoute with LB set uses these upstreams instead of BackendDial.
type LBRoute struct {
	Upstreams []string
	// Weights aligns with Upstreams and is only emitted for the weighted policy.
	Weights []int
	Policy  string
	// CookieName is the sticky-session cookie for the cookie policy.
	CookieName            string
	Retries               int
	TryDuration           string
	TryInterval           string
	ActiveHealth          *ActiveHealthChecks
	PassiveHealth         *PassiveHealthChecks
	StreamCloseDelay      string
	DialTimeout           string
	ResponseHeaderTimeout string
	UpstreamTLS           *UpstreamTLSConfig
	RateLimitRPS          int
	RateLimitBurst        int
}

func (lb *LBRoute) selectionPolicy() *SelectionPolicy {
	switch lb.Policy {
	case "", LBPolicyRoundRobin:
		return nil
	case LBPolicyWeighted:
		if len(lb.Weights) == 0 {
			return nil
		}
		return &SelectionPolicy{Policy: LBPolicyWeighted, Weights: lb.Weights}
	case LBPolicyCookie:
		name := lb.CookieName
		if name == "" {
			name = "lb"
		}
		return &SelectionPolicy{Policy: LBPolicyCookie, Name: name, Fallback: &SelectionPolicy{Policy: LBPolicyRoundRobin}}
	default:
		return &SelectionPolicy{Policy: lb.Policy}
	}
}

// NewLBReverseProxyHandler renders lb as a reverse_proxy handler with a
// multi-upstream pool.
func NewLBReverseProxyHandler(lb *LBRoute) ReverseProxyHandler {
	h := ReverseProxyHandler{Handler: "reverse_proxy", Upstreams: make([]Upstream, len(lb.Upstreams))}
	for i, dial := range lb.Upstreams {
		h.Upstreams[i] = Upstream{Dial: dial}
	}
	if sp := lb.selectionPolicy(); sp != nil || lb.Retries > 0 || lb.TryDuration != "" {
		h.LoadBalancing = &LoadBalancing{SelectionPolicy: sp, Retries: lb.Retries, TryDuration: lb.TryDuration, TryInterval: lb.TryInterval}
	}
	if lb.ActiveHealth != nil || lb.PassiveHealth != nil {
		h.HealthChecks = &HealthChecks{Active: lb.ActiveHealth, Passive: lb.PassiveHealth}
	}
	if lb.UpstreamTLS != nil || lb.DialTimeout != "" || lb.ResponseHeaderTimeout != "" {
		h.Transport = &HTTPTransport{Protocol: "http", TLS: lb.UpstreamTLS, DialTimeout: lb.DialTimeout, ResponseHeaderTimeout: lb.ResponseHeaderTimeout}
	}
	h.StreamCloseDelay = lb.StreamCloseDelay
	return h
}
