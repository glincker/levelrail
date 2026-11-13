package cloudiac

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/GLINCKER/levelrail/internal/cloudconn"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// ConnectionGetter loads a stored cloud connection by name; *cloudconn.Service implements it.
type ConnectionGetter interface {
	Get(ctx context.Context, name string) (cloudconn.Connection, error)
}

// ConnectionAccounts maps a CloudAccount name to the stored connection of the same name,
// and opens that connection's sealed credential only when a step needs it.
type ConnectionAccounts struct {
	Conns  ConnectionGetter
	Sealed cloudconn.SealedReader
}

var (
	_ AccountResolver = ConnectionAccounts{}
	_ OracleResolver  = ConnectionAccounts{}
)

// ResolveAccount implements AccountResolver.
func (c ConnectionAccounts) ResolveAccount(ctx context.Context, name string) (AccountInfo, error) {
	if c.Conns == nil {
		return AccountInfo{}, ErrNotConfigured
	}
	conn, err := c.Conns.Get(ctx, name)
	if err != nil {
		return AccountInfo{}, fmt.Errorf("cloud account %q has no stored connection", name)
	}
	if conn.Provider != cloudconn.ProviderOracle {
		return AccountInfo{}, fmt.Errorf("cloud account %q is not an oracle connection", name)
	}
	return AccountInfo{Region: conn.Config.Region, CompartmentOCID: conn.Config.Compartment,
		Credential: CredentialHandle{ID: conn.Name}}, nil
}

// ResolveOracle implements OracleResolver; the handle id is the connection name.
func (c ConnectionAccounts) ResolveOracle(ctx context.Context, h CredentialHandle) (cloudconn.OracleCredential, error) {
	if c.Conns == nil || c.Sealed == nil {
		return cloudconn.OracleCredential{}, ErrNotConfigured
	}
	conn, err := c.Conns.Get(ctx, h.ID)
	if err != nil {
		return cloudconn.OracleCredential{}, errors.New("cloudiac: credential connection not found")
	}
	return cloudconn.OpenOracleSealed(ctx, c.Sealed, conn)
}

// LiveFetcher builds a Fetcher per call so changed fetch settings apply to the next download.
type LiveFetcher struct {
	Base   FetchConfig
	Limits func() FetchLimits
	mu     sync.Mutex
}

// Ensure implements BinaryEnsurer.
func (l *LiveFetcher) Ensure(ctx context.Context) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cfg := l.Base
	if l.Limits != nil {
		cfg.ApplyLimits(l.Limits())
	}
	f, err := NewFetcher(cfg)
	if err != nil {
		return "", err
	}
	return f.Ensure(ctx)
}

// StringSettings is the string part of the runtime settings service.
type StringSettings interface {
	String(ctx context.Context, key string) string
}

// FetchLimitsFrom reads the fetch limits per call; a nil reader yields zeros, which NewFetcher refuses.
func FetchLimitsFrom(get func() SettingsReader) func() FetchLimits {
	return func() FetchLimits {
		s := get()
		if s == nil {
			return FetchLimits{}
		}
		ctx := context.Background()
		return FetchLimits{
			MaxMB:             s.Int(ctx, runtimesettings.KeyCloudFetchMaxMB),
			MinFreeMB:         s.Int(ctx, runtimesettings.KeyCloudFetchMinFreeMB),
			TimeoutSeconds:    s.Int(ctx, runtimesettings.KeyCloudFetchTimeoutSeconds),
			AllowChecksumOnly: s.Bool(ctx, runtimesettings.KeyCloudAllowChecksumOnly),
		}
	}
}

// LiveSandbox returns a TofuConfig.Live that reads image, user, network and limits per run.
// An empty image or network leaves the field empty, so the plan fails as not configured.
func LiveSandbox(get func() SettingsReader, runtime string) func(TofuConfig) TofuConfig {
	return func(c TofuConfig) TofuConfig {
		c.Runtime = runtime
		s := get()
		ss, ok := s.(StringSettings)
		if s == nil || !ok {
			return c
		}
		ctx := context.Background()
		c.Image = strings.TrimSpace(ss.String(ctx, runtimesettings.KeyCloudEngineImage))
		c.User = strings.TrimSpace(ss.String(ctx, runtimesettings.KeyCloudEngineUser))
		c.Limits = SandboxLimits{
			MemoryMB:   s.Int(ctx, runtimesettings.KeyCloudEngineMemoryMB),
			CPUs:       float64(s.Int(ctx, runtimesettings.KeyCloudEngineCPUs)),
			Pids:       s.Int(ctx, runtimesettings.KeyCloudEnginePids),
			WorkTmpMB:  s.Int(ctx, runtimesettings.KeyCloudEngineRunMinFreeMB),
			StateTmpMB: s.Int(ctx, runtimesettings.KeyCloudEngineTmpMB),
		}
		c.Network = NetworkPolicy{}
		if name := strings.TrimSpace(ss.String(ctx, runtimesettings.KeyCloudEngineNetwork)); name != "" {
			var deny []string
			for _, a := range strings.Split(ss.String(ctx, runtimesettings.KeyCloudEngineDenyAddrs), ",") {
				if a = strings.TrimSpace(a); a != "" {
					deny = append(deny, a)
				}
			}
			if np, err := NewNetworkPolicy(name, deny); err == nil {
				c.Network = np
			}
		}
		return c
	}
}
