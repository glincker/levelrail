package dnszones

import "fmt"

// TokenProviders lists the providers connected with one stored credential, in tie-break order.
var TokenProviders = []string{ProviderDigitalOcean, ProviderHetzner, ProviderAzure}

// IsTokenProvider reports whether name is one of TokenProviders.
func IsTokenProvider(name string) bool {
	for _, n := range TokenProviders {
		if n == name {
			return true
		}
	}
	return false
}

// NewTokenProvider builds the provider called name from its stored credential:
// an API token for DigitalOcean and Hetzner, the credential JSON for Azure.
func NewTokenProvider(name, credential string) (Provider, error) {
	switch name {
	case ProviderDigitalOcean:
		return NewDigitalOcean(credential), nil
	case ProviderHetzner:
		return NewHetzner(credential), nil
	case ProviderAzure:
		return NewAzure(credential)
	}
	return nil, fmt.Errorf("dnszones: unknown provider %q", name)
}
