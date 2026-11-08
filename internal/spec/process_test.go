package spec

import (
	"strings"
	"testing"
)

func TestValidateUserAndWorkingDir(t *testing.T) {
	cases := []struct {
		name    string
		user    string
		dir     string
		wantErr bool
	}{
		{name: "empty keeps image defaults"},
		{name: "name", user: "node"},
		{name: "root allowed like docker", user: "root"},
		{name: "uid gid", user: "1000:1000", dir: "/app"},
		{name: "name group", user: "www-data:www-data", dir: "/var/www/html"},
		{name: "space", user: "node user", wantErr: true},
		{name: "shell chars", user: "1000;rm", wantErr: true},
		{name: "unexpanded variable", user: "${UID}", wantErr: true},
		{name: "two colons", user: "1:2:3", wantErr: true},
		{name: "too long user", user: strings.Repeat("a", defaultUserMaxLen+1), wantErr: true},
		{name: "relative dir", dir: "app", wantErr: true},
		{name: "nul in dir", dir: "/app\x00x", wantErr: true},
		{name: "newline in dir", dir: "/app\nx", wantErr: true},
		{name: "too long dir", dir: "/" + strings.Repeat("a", defaultWorkingDirMaxLen), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Service{User: tc.user, WorkingDir: tc.dir}.validateProcess("web")
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateProcess() error = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil && tc.user != "" && strings.Contains(err.Error(), tc.user) {
				t.Errorf("error echoes the user value: %v", err)
			}
		})
	}
}

func TestValidateUserMaxLenFromEnv(t *testing.T) {
	t.Setenv(envUserMaxLen, "3")
	if err := ValidateUser("node"); err == nil {
		t.Fatal("ValidateUser() accepted a value over the env limit")
	}
}

func TestParseUserAndWorkingDirThroughSchema(t *testing.T) {
	doc := "version: 1\nservices:\n  web:\n    build: {type: image, image: nginx:1}\n    port: 80\n    user: \"1000:1000\"\n    working_dir: /srv\n"
	s, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got := s.Services["web"]; got.User != "1000:1000" || got.WorkingDir != "/srv" {
		t.Fatalf("parsed user/working_dir = %q/%q", got.User, got.WorkingDir)
	}
	bad := strings.Replace(doc, "/srv", "srv", 1)
	if _, err := Parse([]byte(bad)); err == nil {
		t.Fatal("Parse() accepted a relative working_dir")
	}
}
