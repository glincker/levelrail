package platformimport

import "fmt"

// NewSource builds a live source adapter for a platform. The token is the
// Coolify bearer token, Dokploy API key, CapRover password or Heroku API token.
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
	case Heroku:
		return NewHeroku(baseURL, token, o)
	default:
		return nil, fmt.Errorf("unsupported platform %q", platform)
	}
}

// ParsePlatform validates a platform name: a live source or a pasted file.
func ParsePlatform(s string) (Platform, error) {
	switch Platform(s) {
	case Coolify, Dokploy, CapRover, Heroku, Docker, DockerSSH, Compose, Dokku:
		return Platform(s), nil
	default:
		return "", fmt.Errorf("platform must be coolify, dokploy, caprover, heroku, docker, docker-ssh, compose or dokku")
	}
}
