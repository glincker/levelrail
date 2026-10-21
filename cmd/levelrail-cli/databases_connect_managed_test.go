package main

import (
	"slices"
	"testing"
)

func TestConnectStringAndClient(t *testing.T) {
	cases := []struct {
		name    string
		target  connectTarget
		wantStr string
		wantBin string
		wantEnv []string
	}{
		{"postgres", connectTarget{"postgres", "app", "main", "127.0.0.1", 5433}, "postgresql://app@127.0.0.1:5433/main", "psql", []string{"PGPASSWORD=s"}},
		{"mariadb", connectTarget{"mariadb", "root", "", "127.0.0.1", 3307}, "mysql://root@127.0.0.1:3307/", "mysql", []string{"MYSQL_PWD=s"}},
		{"redis", connectTarget{"redis", "", "", "127.0.0.1", 6380}, "redis://127.0.0.1:6380", "redis-cli", []string{"REDISCLI_AUTH=s"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := connectString(tc.target); got != tc.wantStr {
				t.Errorf("connectString = %q, want %q", got, tc.wantStr)
			}
			bin, args, env, ok := clientCommand(tc.target, "s")
			if !ok || bin != tc.wantBin || !slices.Equal(env, tc.wantEnv) {
				t.Errorf("clientCommand = %q %v ok=%v", bin, env, ok)
			}
			if slices.Contains(args, "s") {
				t.Errorf("password leaked into argv: %v", args)
			}
		})
	}
	if _, _, _, ok := clientCommand(connectTarget{engine: "mongo"}, ""); ok {
		t.Error("unknown engine should have no client")
	}
}
