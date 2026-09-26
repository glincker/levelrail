package platformimport

import "fmt"

// NewSource builds a live source adapter for a platform. The token is the
// Coolify bearer token, Dokploy API key or CapRover password.
func NewSource(platform Platform, baseURL, token string, o ClientOptions) (Source, error) {
	if token == "" {
		return nil, fmt.Errorf("a token is required")
	}
	switch platform {
	case Coolify:
		return NewCoolify(baseURL, token, o)
	case Dokploy:
		return NewDokploy(baseURL, token, o)
	case CapRover:
		return NewCapRover(baseURL, token, o)
	default:
		return nil, fmt.Errorf("unsupported platform %q", platform)
	}
}

// ParsePlatform validates a live-source platform name.
func ParsePlatform(s string) (Platform, error) {
	switch Platform(s) {
	case Coolify, Dokploy, CapRover:
		return Platform(s), nil
	default:
		return "", fmt.Errorf("platform must be coolify, dokploy or caprover")
	}
}
