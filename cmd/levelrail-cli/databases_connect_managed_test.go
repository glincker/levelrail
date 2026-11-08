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
		{"postgres", connectTarget{"postgres", "app", "main", "127.0.0.1", 5433, false}, "postgresql://app@127.0.0.1:5433/main", "psql", []string{"PGPASSWORD=s"}},
		{"mariadb", connectTarget{"mariadb", "root", "", "127.0.0.1", 3307, false}, "mysql://root@127.0.0.1:3307/", "mysql", []string{"MYSQL_PWD=s"}},
		{"redis", connectTarget{"redis", "", "", "127.0.0.1", 6380, false}, "redis://127.0.0.1:6380", "redis-cli", []string{"REDISCLI_AUTH=s"}},
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

func TestConnectStringRedisFamilyTLS(t *testing.T) {
	cases := []struct {
		engine   string
		tls      bool
		wantStr  string
		wantTLSF bool
	}{
		{"redis", false, "redis://127.0.0.1:7000", false},
		{"redis", true, "rediss://127.0.0.1:7000", true},
		{"valkey", false, "redis://127.0.0.1:7000", false},
		{"valkey", true, "rediss://127.0.0.1:7000", true},
	}
	for _, tc := range cases {
		t.Run(tc.engine, func(t *testing.T) {
			target := connectTarget{engine: tc.engine, host: "127.0.0.1", port: 7000, tls: tc.tls}
			if got := connectString(target); got != tc.wantStr {
				t.Errorf("connectString = %q, want %q", got, tc.wantStr)
			}
			_, args, _, ok := clientCommand(target, "")
			if !ok || slices.Contains(args, "--tls") != tc.wantTLSF {
				t.Errorf("clientCommand args = %v ok=%v, want tls flag %v", args, ok, tc.wantTLSF)
			}
		})
	}
}
