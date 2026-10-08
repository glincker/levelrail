package githubapp

import "testing"

func TestNewClientUsesGuardedHTTP(t *testing.T) {
	c := NewClient()
	if c.HTTP == nil || c.HTTP.Timeout == 0 {
		t.Fatalf("NewClient HTTP = %+v, want guarded client with timeout", c.HTTP)
	}
}
