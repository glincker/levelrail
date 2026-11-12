package cloudconn

import (
	"context"
	"sync"
)

// Fake is an in-memory provider for tests: it validates like the real config rules
// and returns a scripted class, recording the credential length it was handed.
type Fake struct {
	ProviderName string
	Class        string
	Err          error
	Regions      []string

	mu      sync.Mutex
	Checked []string
	CredLen []int
}

// Name returns the provider name the fake stands in for.
func (f *Fake) Name() string { return f.ProviderName }

// ValidateConfig applies the shared per provider rules.
func (f *Fake) ValidateConfig(cfg Config, authMethod string) error {
	return validateProviderConfig(f.ProviderName, cfg, authMethod)
}

// Check returns the scripted class.
func (f *Fake) Check(_ context.Context, t Target) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Checked = append(f.Checked, t.Connection.Name)
	f.CredLen = append(f.CredLen, t.Credential.Len())
	if f.Class == "" {
		return ClassOK, f.Err
	}
	return f.Class, f.Err
}

// ListRegions returns the scripted regions.
func (f *Fake) ListRegions(context.Context, Target) ([]string, error) {
	return f.Regions, f.Err
}

// FakeRegistry maps every provider name to a Fake sharing one script.
func FakeRegistry(class string, err error) (Registry, map[string]*Fake) {
	r, fakes := Registry{}, map[string]*Fake{}
	for n := range DefaultRegistry() {
		f := &Fake{ProviderName: n, Class: class, Err: err}
		r[n], fakes[n] = f, f
	}
	return r, fakes
}
