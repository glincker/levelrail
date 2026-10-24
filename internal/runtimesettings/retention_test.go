package runtimesettings

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestTokenRetentionPresets(t *testing.T) {
	var def Definition
	for _, d := range Definitions() {
		if d.Key == KeyRevokedTokenDays {
			def = d
		}
	}
	if def.Key == "" {
		t.Fatal("revoked token retention is not registered")
	}
	tests := []struct {
		raw     string
		want    string
		wantErr bool
	}{
		{"0", "0", false},
		{"30", "30", false},
		{"90", "90", false},
		{"365", "365", false},
		{`"60"`, "60", false},
		{"45", "", true},
		{"-1", "", true},
		{"1.5", "", true},
		{`"soon"`, "", true},
	}
	for _, tc := range tests {
		got, err := ParseJSON(def, json.RawMessage(tc.raw))
		var verr *ValidationError
		switch {
		case tc.wantErr && !errors.As(err, &verr):
			t.Errorf("%s: err = %v, want a field validation error", tc.raw, err)
		case !tc.wantErr && (err != nil || got != tc.want):
			t.Errorf("%s: got %q, %v, want %q", tc.raw, got, err, tc.want)
		}
	}
}

func TestTokenRetentionDefaultAndInvalidEnv(t *testing.T) {
	env := map[string]string{"APP_RETENTION_REVOKED_TOKEN_DAYS": "45"}
	svc := New(nil, func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if got := svc.Int(context.Background(), KeyRevokedTokenDays); got != 90 {
		t.Errorf("invalid env preset resolved to %d, want the default 90", got)
	}
	env["APP_RETENTION_REVOKED_TOKEN_DAYS"] = "0"
	if got := svc.Int(context.Background(), KeyRevokedTokenDays); got != 0 {
		t.Errorf("env 0 resolved to %d, want 0 (keep forever)", got)
	}
}
