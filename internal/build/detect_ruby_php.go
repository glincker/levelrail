package build

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Railpack's ruby start command binds ${PORT:-3000} and its php image
// serves Caddy on :${PORT:-80}, and the platform does not inject PORT,
// so these are the ports a default build really listens on.
const (
	defaultRubyPort = 3000
	defaultPHPPort  = 80
)

var pumaPortRe = regexp.MustCompile(`(?m)^\s*port\s+(?:ENV\.fetch\(\s*["']PORT["']\s*(?:,\s*|\)\s*\{\s*)|ENV\[["']PORT["']\]\s*\|\|\s*)?(\d+)`)

// detectRubyPort returns the port a Ruby app listens on: an explicit port
// in the Procfile web line, then config/puma.rb, then Railpack's default.
func detectRubyPort(dir string) (int, string) {
	if p := procfileWebPort(dir); p > 0 {
		return p, "from Procfile web command"
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "config", "puma.rb")); err == nil { //nolint:gosec // dir is this call's own freshly cloned temp checkout
		if m := pumaPortRe.FindStringSubmatch(string(raw)); len(m) > 1 {
			if p, err := strconv.Atoi(m[1]); err == nil && p > 0 && p <= 65535 {
				return p, "from config/puma.rb"
			}
		}
	}
	return defaultRubyPort, "Railpack ruby default"
}

// detectPHPPort returns the port a PHP app listens on. Railpack's php
// provider fronts the app with FrankenPHP/Caddy, which listens on 80.
func detectPHPPort(dir string) (int, string) {
	if p := procfileWebPort(dir); p > 0 {
		return p, "from Procfile web command"
	}
	return defaultPHPPort, "Railpack php default"
}

func procfileWebPort(dir string) int {
	raw, err := os.ReadFile(filepath.Join(dir, "Procfile")) //nolint:gosec // dir is this call's own freshly cloned temp checkout
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(raw), "\n") {
		name, cmd, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok && strings.TrimSpace(name) == "web" {
			return extractPort(cmd)
		}
	}
	return 0
}
