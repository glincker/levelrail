package authengine

import (
	"slices"
	"testing"
)

func TestSignInApprove_TokenOnly(t *testing.T) {
	tests := []struct {
		name string
		ok   bool
	}{
		{"device logins never request it", !slices.Contains(EngineAbilities(), AbilitySignInApprove)},
		{"a minted token may carry it", func() bool {
			m, err := MapAbilities([]string{AbilitySignInApprove})
			return err == nil && slices.Equal(m, []string{AbilitySignInApprove})
		}()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.ok {
				t.Fatal("check failed")
			}
		})
	}
}
