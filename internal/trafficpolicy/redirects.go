package trafficpolicy

// Trailing slash normalisation modes.
const (
	SlashOff    = "off"
	SlashAdd    = "add"
	SlashRemove = "remove"
)

// Redirects is a domain's request normalisation: force HTTPS, trailing
// slash and lower case host. Canonical host and alias redirects live in the
// domain_redirect table, one row per redirected host.
type Redirects struct {
	ForceHTTPS bool `json:"force_https"`
	// ForceHTTPSStatus is 308 (default, keeps the method) or 301.
	ForceHTTPSStatus int    `json:"force_https_status,omitempty"`
	TrailingSlash    string `json:"trailing_slash,omitempty"`
	LowercaseHost    bool   `json:"lowercase_host,omitempty"`
}

// HTTPSStatus returns the effective force HTTPS status.
func (r Redirects) HTTPSStatus() int {
	if r.ForceHTTPSStatus == 0 {
		return 308
	}
	return r.ForceHTTPSStatus
}

// SlashMode returns the effective trailing slash mode.
func (r Redirects) SlashMode() string {
	if r.TrailingSlash == "" {
		return SlashOff
	}
	return r.TrailingSlash
}

// Validate checks the settings.
func (r *Redirects) Validate() error {
	col := &collector{kind: KindRedirects}
	if r.ForceHTTPSStatus != 0 && r.ForceHTTPSStatus != 301 && r.ForceHTTPSStatus != 308 {
		col.add("force_https_status", "must be 301 or 308")
	}
	switch r.TrailingSlash {
	case "", SlashOff, SlashAdd, SlashRemove:
	default:
		col.add("trailing_slash", "must be off, add or remove")
	}
	return col.err()
}

// AliasStatuses are the redirect codes an alias or canonical redirect may use.
var AliasStatuses = map[int]bool{301: true, 302: true, 307: true, 308: true}
