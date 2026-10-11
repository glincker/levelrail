package api

import "testing"

func TestDoctorCheckSecretReveal(t *testing.T) {
	tests := []struct {
		policy, tokens, want string
	}{
		{"", "", doctorStatusOK},
		{"step_up", "on", doctorStatusOK},
		{"disabled", "", doctorStatusOK},
		{"allow", "", doctorStatusWarn},
		{"alow", "", doctorStatusWarn},
	}
	for _, tt := range tests {
		t.Run(tt.policy+"/"+tt.tokens, func(t *testing.T) {
			t.Setenv(envSecretReveal, tt.policy)
			t.Setenv(envSecretRevealTokens, tt.tokens)
			got := doctorCheckSecretReveal()
			if got.Status != tt.want || got.Code != "secret_reveal_policy" || got.Message == "" {
				t.Fatalf("check = %+v", got)
			}
		})
	}
}
