package ingress

import (
	"reflect"
	"testing"
)

func TestIsWildcardDomain(t *testing.T) {
	tests := []struct {
		domain string
		want   bool
	}{
		{"*.example.com", true},
		{"*.sub.example.com", true},
		{"example.com", false},
		{"sub.example.com", false},
		{"", false},
		{"a*.example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			if got := IsWildcardDomain(tt.domain); got != tt.want {
				t.Errorf("IsWildcardDomain(%q) = %v, want %v", tt.domain, got, tt.want)
			}
		})
	}
}

func TestValidateWildcardDomain(t *testing.T) {
	tests := []struct {
		name    string
		domain  string
		wantErr bool
	}{
		{"plain domain, no wildcard", "example.com", false},
		{"valid wildcard", "*.example.com", false},
		{"valid wildcard, deeper base", "*.sub.example.com", false},
		{"wildcard not in leading position", "sub.*.example.com", true},
		{"wildcard mid-label", "a*.example.com", true},
		{"double wildcard", "*.*.example.com", true},
		{"wildcard with empty base", "*.", true},
		{"wildcard with single-label base", "*.internal", true},
		{"empty string", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWildcardDomain(tt.domain)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWildcardDomain(%q) error = %v, wantErr %v", tt.domain, err, tt.wantErr)
			}
		})
	}
}

func TestSplitWildcardHosts(t *testing.T) {
	tests := []struct {
		name         string
		hosts        []string
		wantWildcard []string
		wantRegular  []string
	}{
		{
			name:         "mixed",
			hosts:        []string{"app.example.com", "*.example.com", "other.example.com"},
			wantWildcard: []string{"*.example.com"},
			wantRegular:  []string{"app.example.com", "other.example.com"},
		},
		{
			name:         "all wildcard",
			hosts:        []string{"*.a.com", "*.b.com"},
			wantWildcard: []string{"*.a.com", "*.b.com"},
			wantRegular:  nil,
		},
		{
			name:         "all regular",
			hosts:        []string{"a.com", "b.com"},
			wantWildcard: nil,
			wantRegular:  []string{"a.com", "b.com"},
		},
		{
			name:         "empty",
			hosts:        nil,
			wantWildcard: nil,
			wantRegular:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotWildcard, gotRegular := splitWildcardHosts(tt.hosts)
			if !reflect.DeepEqual(gotWildcard, tt.wantWildcard) {
				t.Errorf("splitWildcardHosts() wildcard = %v, want %v", gotWildcard, tt.wantWildcard)
			}
			if !reflect.DeepEqual(gotRegular, tt.wantRegular) {
				t.Errorf("splitWildcardHosts() regular = %v, want %v", gotRegular, tt.wantRegular)
			}
		})
	}
}
