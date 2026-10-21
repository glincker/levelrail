package main

import (
	"strings"
	"testing"
)

func TestValidateStateDir(t *testing.T) {
	cases := []struct {
		dir string
		ok  bool
	}{
		{"/var/lib/x-migrate", true},
		{"/srv/migrate_state.d", true},
		{"", false},
		{"/", false},
		{"relative/dir", false},
		{".", false},
		{"/var/lib/../..", false},
		{"/var/lib/x/", false},
		{"//var/lib/x", false},
		{"/var/lib/with space", false},
		{"/var/lib/x\nExecStart=/bin/sh", false},
		{"/var/lib/x%h", false},
		{`/var/lib/"x"`, false},
	}
	for _, tc := range cases {
		t.Run(tc.dir, func(t *testing.T) {
			if err := validateStateDir(tc.dir); (err == nil) != tc.ok {
				t.Fatalf("validateStateDir(%q)=%v want ok=%v", tc.dir, err, tc.ok)
			}
		})
	}
}

func TestRenderUnitQuotesEnvironmentAndAddsSandboxing(t *testing.T) {
	u := renderUnit("/usr/local/bin/x-migrate", `cp.example:9443" Environment="EVIL=1`, "/var/lib/x-migrate")
	if strings.Contains(u, "\nEnvironment=\"EVIL=1") || !strings.Contains(u, `Environment="APP_CONTROL_PLANE_ADDR=cp.example:9443\" Environment=\"EVIL=1"`) {
		t.Fatalf("environment value is not quoted and escaped:\n%s", u)
	}
	if !strings.Contains(u, `Environment="APP_MIGRATE_STATE_DIR=/var/lib/x-migrate"`) {
		t.Fatalf("state dir environment is not quoted:\n%s", u)
	}
	for _, want := range []string{
		"RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6", "SystemCallFilter=@system-service", "PrivateDevices=yes",
		"ProtectKernelTunables=yes", "ProtectKernelModules=yes", "MemoryDenyWriteExecute=yes", "LockPersonality=yes", "ProtectProc=invisible",
	} {
		if !strings.Contains(u, want) {
			t.Errorf("unit is missing %q", want)
		}
	}
}
