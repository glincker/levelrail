package cliupdate

import "testing"

func envOf(v string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		if k == EnvInstallMethod && v != "" {
			return v, true
		}
		return "", false
	}
}

func TestDetectMethod(t *testing.T) {
	cases := []struct{ name, path, env, want string }{
		{"plain binary", "/usr/local/bin/levelrail-cli", "", MethodBinary},
		{"launcher env", "/opt/x/levelrail-cli", "npm", MethodNPM},
		{"npm path", "/usr/lib/node_modules/levelrail-cli-linux-amd64/bin/levelrail-cli", "", MethodNPM},
		{"pnpm path", "/h/.local/share/pnpm/global/5/node_modules/.pnpm/a/node_modules/levelrail-cli-linux-amd64/bin/x", "", MethodPNPM},
		{"yarn path", "/h/.config/yarn/global/node_modules/levelrail-cli-linux-amd64/bin/x", "", MethodYarn},
		{"npx cache", "/h/.npm/_npx/ab/node_modules/levelrail-cli-linux-amd64/bin/x", "npm", MethodNPX},
		{"pnpm refines npm env", "/h/node_modules/.pnpm/a/node_modules/p/bin/x", "npm", MethodPNPM},
		{"brew", "/opt/homebrew/Cellar/levelrail-cli/1/bin/levelrail-cli", "", MethodBrew},
		{"env wins for binary", "/h/node_modules/p/bin/x", "binary", MethodBinary},
		{"unknown env ignored", "/usr/local/bin/x", "bogus", MethodBinary},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DetectMethod(c.path, envOf(c.env)); got != c.want {
				t.Fatalf("got %s want %s", got, c.want)
			}
		})
	}
}

func TestUpgradeCommand(t *testing.T) {
	cases := []struct{ method, want string }{
		{MethodNPM, "npm install --global levelrail-cli@latest"},
		{MethodPNPM, "pnpm add --global levelrail-cli@latest"},
		{MethodYarn, "yarn global add levelrail-cli@latest"},
		{MethodBrew, "brew upgrade levelrail-cli"},
		{MethodBinary, "levelrail-cli update"},
	}
	for _, c := range cases {
		if got := UpgradeCommand(c.method, "levelrail-cli"); got != c.want {
			t.Errorf("%s: got %q want %q", c.method, got, c.want)
		}
	}
	if !PackageManaged(MethodNPX) || PackageManaged(MethodBrew) || PackageManaged(MethodBinary) {
		t.Error("PackageManaged wrong")
	}
}

func TestMinorBehind(t *testing.T) {
	cases := []struct {
		cli, server string
		want        bool
	}{
		{"v0.9.3", "v0.10.0", true},
		{"v0.9.3", "v0.9.9", false},
		{"v0.9.3", "v0.9.3", false},
		{"v0.10.0", "v0.9.3", false},
		{"v0.9.3", "v1.0.0", true},
		{"dev", "v0.9.3", false},
		{"v0.9.3", "", false},
	}
	for _, c := range cases {
		if got := MinorBehind(c.cli, c.server); got != c.want {
			t.Errorf("%s vs %s: got %v", c.cli, c.server, got)
		}
	}
}

func TestUpgradeArgv(t *testing.T) {
	if got := UpgradeArgv(MethodYarn, "1.2.3"); len(got) != 4 || got[3] != "levelrail-cli@1.2.3" {
		t.Fatalf("got %v", got)
	}
	if UpgradeArgv(MethodNPX, "latest") != nil || UpgradeArgv(MethodBrew, "latest") != nil {
		t.Fatal("npx and brew have no argv")
	}
}
