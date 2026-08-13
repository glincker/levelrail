package api

import "testing"

func TestHasAbility(t *testing.T) {
	tests := []struct {
		name      string
		abilities []string
		required  string
		want      bool
	}{
		{name: "direct match", abilities: []string{AbilityRead}, required: AbilityRead, want: true},
		{name: "no match", abilities: []string{AbilityRead}, required: AbilityWrite, want: false},
		{name: "root implies read", abilities: []string{AbilityRoot}, required: AbilityRead, want: true},
		{name: "root implies deploy", abilities: []string{AbilityRoot}, required: AbilityDeploy, want: true},
		{name: "empty abilities", abilities: nil, required: AbilityRead, want: false},
		{name: "multiple abilities, one matches", abilities: []string{AbilityRead, AbilityDeploy}, required: AbilityDeploy, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasAbility(tt.abilities, tt.required); got != tt.want {
				t.Errorf("hasAbility(%v, %q) = %v, want %v", tt.abilities, tt.required, got, tt.want)
			}
		})
	}
}

func TestValidateAbilities(t *testing.T) {
	tests := []struct {
		name      string
		abilities []string
		wantErr   bool
	}{
		{name: "single valid", abilities: []string{AbilityRead}, wantErr: false},
		{name: "multiple valid", abilities: []string{AbilityRead, AbilityDeploy}, wantErr: false},
		{name: "root alone", abilities: []string{AbilityRoot}, wantErr: false},
		{name: "empty", abilities: nil, wantErr: true},
		{name: "root combined with another", abilities: []string{AbilityRoot, AbilityRead}, wantErr: true},
		{name: "unknown ability", abilities: []string{"delete-everything"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAbilities(tt.abilities)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAbilities(%v) error = %v, wantErr %v", tt.abilities, err, tt.wantErr)
			}
		})
	}
}
