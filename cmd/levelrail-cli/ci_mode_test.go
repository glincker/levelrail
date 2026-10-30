package main

import (
	"strings"
	"testing"
)

func fakeEnv(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestEnvAliasPrecedence(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		key  string
		want string
	}{
		{"new wins over legacy", map[string]string{"LEVELRAIL_TOKEN": "new", "APP_API_TOKEN": "old"}, "APP_API_TOKEN", "new"},
		{"legacy used when new unset", map[string]string{"APP_API_TOKEN": "old"}, "APP_API_TOKEN", "old"},
		{"empty new falls back", map[string]string{"LEVELRAIL_TOKEN": "", "APP_API_TOKEN": "old"}, "APP_API_TOKEN", "old"},
		{"url alias", map[string]string{"LEVELRAIL_API_URL": "https://a", "APP_API_URL": "https://b"}, "APP_API_URL", "https://a"},
		{"profile alias", map[string]string{"LEVELRAIL_PROFILE": "ci"}, "APP_PROFILE", "ci"},
		{"unset stays unset", map[string]string{}, "APP_API_TOKEN", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := envAliasLookup(fakeEnv(tc.env))(tc.key)
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestFlagBeatsEnvToken(t *testing.T) {
	look := envAliasLookup(fakeEnv(map[string]string{"LEVELRAIL_TOKEN": "env"}))
	if got := resolveToken("flag", look, "levelrail-cli", "p"); got != "flag" {
		t.Fatalf("got %q", got)
	}
	if got := resolveToken("", look, "levelrail-cli", "p"); got != "env" {
		t.Fatalf("got %q", got)
	}
}

func TestCIDetectionAndColor(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		ci, color bool
	}{
		{"none", map[string]string{}, false, false},
		{"CI true", map[string]string{"CI": "true"}, true, true},
		{"CI false", map[string]string{"CI": "false"}, false, false},
		{"LEVELRAIL_CI", map[string]string{"LEVELRAIL_CI": "1"}, true, true},
		{"NO_COLOR only", map[string]string{"NO_COLOR": "1"}, false, true},
		{"empty NO_COLOR ignored", map[string]string{"NO_COLOR": ""}, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ci := ciEnv(fakeEnv(tc.env))
			if ci != tc.ci || colorDisabled(fakeEnv(tc.env), ci) != tc.color {
				t.Fatalf("ci=%v color=%v", ci, colorDisabled(fakeEnv(tc.env), ci))
			}
		})
	}
}

func TestRequireInputInCI(t *testing.T) {
	old := ciMode
	defer func() { ciMode = old }()
	ciMode = false
	if err := requireInput("login", "pass --password"); err != nil {
		t.Fatal(err)
	}
	ciMode = true
	err := requireInput("login", "pass --password")
	if err == nil || !strings.Contains(err.Error(), "--password") {
		t.Fatalf("got %v", err)
	}
	_, _, err = resolveLoginCredentials("", "", strings.NewReader("u\n"), &strings.Builder{}, func() (string, error) { return "x", nil })
	if err == nil || !strings.Contains(err.Error(), "--username") {
		t.Fatalf("got %v", err)
	}
	_, _, err = resolveLoginCredentials("u", "", strings.NewReader(""), &strings.Builder{}, func() (string, error) { return "x", nil })
	if err == nil || !strings.Contains(err.Error(), "--password") {
		t.Fatalf("got %v", err)
	}
}

func TestCredentialSource(t *testing.T) {
	old := rawLookupEnv
	defer func() { rawLookupEnv = old }()
	tests := []struct {
		name string
		flag string
		env  map[string]string
		want string
	}{
		{"flag", "t", map[string]string{"LEVELRAIL_TOKEN": "x"}, "flag --token"},
		{"new env", "", map[string]string{"LEVELRAIL_TOKEN": "x", "APP_API_TOKEN": "y"}, "env LEVELRAIL_TOKEN"},
		{"legacy env", "", map[string]string{"APP_API_TOKEN": "y"}, "env APP_API_TOKEN"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rawLookupEnv = fakeEnv(tc.env)
			got := credentialSource(tc.flag, "levelrail-cli-test", "p")
			if got != tc.want || strings.Contains(got, "x") && !strings.Contains(tc.want, "LEVELRAIL") && !strings.Contains(tc.want, "flag") {
				t.Fatalf("got %q", got)
			}
		})
	}
}
