package vault

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// Tunables are the live Vault client bounds, read per resolve so a settings change applies at once.
type Tunables struct {
	Timeout      time.Duration
	CacheTTL     time.Duration
	AllowPrivate bool
	RenewPercent int
}

// TunablesFrom reads the bounds from runtime settings; a nil service serves env then defaults.
func TunablesFrom(s *runtimesettings.Service) func(context.Context) Tunables {
	return func(ctx context.Context) Tunables {
		return Tunables{
			Timeout:      time.Duration(s.Int(ctx, runtimesettings.KeySecretProviderTimeout)) * time.Second,
			CacheTTL:     time.Duration(s.Int(ctx, runtimesettings.KeyVaultCacheTTL)) * time.Second,
			AllowPrivate: s.Bool(ctx, runtimesettings.KeyVaultAllowPrivate),
			RenewPercent: s.Int(ctx, runtimesettings.KeyVaultTokenRenewAfter),
		}
	}
}
