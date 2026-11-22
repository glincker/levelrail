package dnszones

import "fmt"

// TokenProviders lists the providers connected with one stored credential, in tie-break order.
var TokenProviders = []string{ProviderDigitalOcean, ProviderHetzner, ProviderAzure, ProviderPorkbun, ProviderDeSEC, ProviderGoDaddy, ProviderNamecheap}

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
// an API token for DigitalOcean, Hetzner and deSEC, the credential JSON for Azure,
// and key:secret (or JSON) for Porkbun, key:secret for GoDaddy and apiuser:apikey:clientip for Namecheap.
func NewTokenProvider(name, credential string) (Provider, error) {
	switch name {
	case ProviderDigitalOcean:
		return NewDigitalOcean(credential), nil
	case ProviderHetzner:
		return NewHetzner(credential), nil
	case ProviderAzure:
		return NewAzure(credential)
	case ProviderPorkbun:
		return NewPorkbun(credential)
	case ProviderDeSEC:
		return NewDeSEC(credential), nil
	case ProviderGoDaddy:
		return NewGoDaddy(credential)
	case ProviderNamecheap:
		return NewNamecheap(credential)
	}
	return nil, fmt.Errorf("dnszones: unknown provider %q", name)
}
