package authengine

import (
	"reflect"
	"testing"
)

func TestParseAreas(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    []Area
		wantErr bool
	}{
		{"empty means all", "", AllAreas, false},
		{"blank means all", "  ", AllAreas, false},
		{"single", "tokens", []Area{AreaTokens}, false},
		{"several keep order", "oauth, tokens", []Area{AreaOAuth, AreaTokens}, false},
		{"case and spaces", " Tokens ,DEVICE", []Area{AreaTokens, AreaDevice}, false},
		{"duplicates collapse", "tokens,tokens", []Area{AreaTokens}, false},
		{"trailing comma", "mfa,", []Area{AreaMFA}, false},
		{"unknown fails", "tokens,billing", nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseAreas(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestAreaActive(t *testing.T) {
	tests := []struct {
		name   string
		engine string
		areas  string
		area   Area
		want   bool
	}{
		{"legacy never active", EngineLegacy, "", AreaTokens, false},
		{"library all by default", EngineLibrary, "", AreaOAuth, true},
		{"library listed", EngineLibrary, "tokens,device", AreaDevice, true},
		{"library not listed", EngineLibrary, "tokens,device", AreaSessions, false},
		{"legacy ignores list", EngineLegacy, "tokens", AreaTokens, false},
		{"bad list is inactive", EngineLibrary, "nope", AreaTokens, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvEngine, tc.engine)
			t.Setenv(EnvAreas, tc.areas)
			if got := AreaActive(tc.area); got != tc.want {
				t.Fatalf("AreaActive(%s) = %v, want %v", tc.area, got, tc.want)
			}
		})
	}
}

func TestValidateAreas(t *testing.T) {
	t.Setenv(EnvAreas, "tokens")
	if err := ValidateAreas(); err != nil {
		t.Fatalf("valid list rejected: %v", err)
	}
	t.Setenv(EnvAreas, "tokens,bogus")
	if err := ValidateAreas(); err == nil {
		t.Fatal("unknown area accepted")
	}
}
