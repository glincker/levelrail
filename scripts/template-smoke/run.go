package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

const (
	statusPass = "pass"
	statusFail = "fail"
	statusSkip = "skip"

	logTailLines    = 15
	teardownTimeout = 2 * time.Minute
	inspectTimeout  = time.Minute
)

type serviceResult struct {
	Name     string   `json:"name"`
	Image    string   `json:"image,omitempty"`
	State    string   `json:"state"`
	ExitCode int      `json:"exit_code,omitempty"`
	Health   string   `json:"health,omitempty"`
	Logs     []string `json:"logs,omitempty"`
}

type result struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Category string          `json:"category"`
	Status   string          `json:"status"`
	Summary  string          `json:"summary"`
	Services []serviceResult `json:"services,omitempty"`
	Seconds  float64         `json:"seconds"`
}

var unsafeProject = regexp.MustCompile(`[^a-z0-9_-]+`)

// smokeTemplate runs one template end to end and always tears down what
// it created, even when its budget ran out.
func smokeTemplate(ctx context.Context, t catalog.Template, o options) (r result) {
	start := time.Now()
	r = result{ID: t.ID, Name: t.Name, Category: t.Category}
	defer func() { r.Seconds = time.Since(start).Round(100 * time.Millisecond).Seconds() }()
	if t.RequiresGPU {
		return skipped(r, "needs a GPU")
	}

	dir, err := os.MkdirTemp("", "lrsmoke-")
	if err != nil {
		return failed(r, "temp dir: "+err.Error())
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err := writeProject(dir, t); err != nil {
		return failed(r, err.Error())
	}
	p := project{dir: dir, name: unsafeProject.ReplaceAllString(strings.ToLower(o.project+"-"+t.ID), "-"), override: true, subnet: nextSubnet(o.subnetBase)}
	volumes := namedVolumes(t.Compose)
	if err := p.writeOverride(volumes, nil); err != nil {
		return failed(r, err.Error())
	}

	tctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()

	services, err := p.lines(tctx, "config", "--services")
	if err != nil {
		return failed(r, "compose config: "+err.Error())
	}
	images, err := p.lines(tctx, "config", "--images")
	if err != nil {
		return failed(r, "compose config: "+err.Error())
	}
	if reason, err := ensureImages(tctx, images, o); err != nil {
		return failed(r, err.Error())
	} else if reason != "" {
		return skipped(r, reason)
	}
	if err := p.writeOverride(volumes, services); err != nil {
		return failed(r, err.Error())
	}

	defer p.down()
	_, upErr := p.run(tctx, "up", "-d", "--no-build", "--pull", "never", "--quiet-pull")
	select {
	case <-tctx.Done():
	case <-time.After(o.settle):
	}
	// A budget spent on pulls must not also lose the result.
	ictx, icancel := context.WithTimeout(context.Background(), inspectTimeout)
	defer icancel()
	r.Services = p.inspect(ictx, services)
	return classify(r, upErr)
}

func skipped(r result, why string) result {
	r.Status, r.Summary = statusSkip, why
	return r
}

func failed(r result, why string) result {
	r.Status, r.Summary = statusFail, why
	return r
}

// classify is pass only when every service is up and none is unhealthy.
func classify(r result, upErr error) result {
	var problems []string
	starting := false
	for _, s := range r.Services {
		switch {
		case s.State == "missing":
			problems = append(problems, s.Name+" was never created")
		case s.State == "exited" || s.State == "dead":
			problems = append(problems, fmt.Sprintf("%s exited with code %d", s.Name, s.ExitCode))
		case s.State != "running":
			problems = append(problems, s.Name+" is "+s.State)
		case s.Health == "unhealthy":
			problems = append(problems, s.Name+" healthcheck failing")
		case s.Health == "starting":
			starting = true
		}
	}
	if upErr != nil {
		problems = append([]string{"compose up: " + upErr.Error()}, problems...)
	}
	if len(problems) > 0 {
		return failed(r, strings.Join(problems, "; "))
	}
	r.Status, r.Summary = statusPass, "all services running"
	if starting {
		r.Summary += " (healthcheck still starting)"
	}
	return r
}

// overrideFor drops host ports and fixed names so runs never collide, and
// disables Docker restarts so an exit stays visible. It also declares the
// named volumes a deploy creates implicitly but docker compose requires.
func overrideFor(volumes, services []string, subnet string) string {
	var b strings.Builder
	if subnet != "" {
		fmt.Fprintf(&b, "networks:\n  default:\n    ipam:\n      config:\n        - subnet: %s\n", subnet)
	}
	if len(services) > 0 {
		b.WriteString("services:\n")
	}
	for _, s := range services {
		fmt.Fprintf(&b, "  %s:\n    ports: !reset []\n    container_name: !reset null\n    restart: \"no\"\n", s)
	}
	if len(volumes) > 0 {
		b.WriteString("volumes:\n")
	}
	for _, v := range volumes {
		fmt.Fprintf(&b, "  %s: {}\n", v)
	}
	return b.String()
}

var volumeRef = regexp.MustCompile(`(?m)^\s*-\s*["']?([A-Za-z0-9][A-Za-z0-9_.-]*):/|^\s*source:\s*["']?([A-Za-z0-9][A-Za-z0-9_.-]*)["']?\s*$`)

func namedVolumes(composeBody string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range volumeRef.FindAllStringSubmatch(composeBody, -1) {
		name := m[1] + m[2]
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

func (p project) writeOverride(volumes, services []string) error {
	if err := os.WriteFile(filepath.Join(p.dir, "override.yaml"), []byte(overrideFor(volumes, services, p.subnet)), 0o600); err != nil {
		return fmt.Errorf("write override: %w", err)
	}
	return nil
}

type project struct {
	dir      string
	name     string
	override bool
	subnet   string
}

var subnetSeq atomic.Uint32

// nextSubnet gives each run its own /24 for the default network, so many
// runs never exhaust Docker's predefined address pools.
func nextSubnet(base string) string {
	n := subnetSeq.Add(1) % 256
	return fmt.Sprintf("%s.%d.0/24", base, n)
}

func (p project) args(extra ...string) []string {
	a := []string{"compose", "-p", p.name, "--project-directory", p.dir, "-f", filepath.Join(p.dir, "compose.yaml")}
	if p.override {
		a = append(a, "-f", filepath.Join(p.dir, "override.yaml"))
	}
	return append(a, extra...)
}

func (p project) run(ctx context.Context, extra ...string) (string, error) {
	return docker(ctx, p.args(extra...)...)
}

func (p project) lines(ctx context.Context, extra ...string) ([]string, error) {
	out, err := p.run(ctx, extra...)
	if err != nil {
		return nil, err
	}
	return splitLines(out), nil
}

func (p project) down() {
	ctx, cancel := context.WithTimeout(context.Background(), teardownTimeout)
	defer cancel()
	_, _ = p.run(ctx, "down", "-v", "--remove-orphans", "-t", "3")
}

type psEntry struct {
	Service  string `json:"Service"`
	Image    string `json:"Image"`
	State    string `json:"State"`
	Health   string `json:"Health"`
	ExitCode int    `json:"ExitCode"`
}

func (p project) inspect(ctx context.Context, services []string) []serviceResult {
	out, _ := p.run(ctx, "ps", "-a", "--format", "json")
	byService := map[string]psEntry{}
	for _, e := range parsePS(out) {
		byService[e.Service] = e
	}
	res := make([]serviceResult, 0, len(services))
	for _, s := range services {
		e, ok := byService[s]
		if !ok {
			res = append(res, serviceResult{Name: s, State: "missing"})
			continue
		}
		sr := serviceResult{Name: s, Image: e.Image, State: e.State, ExitCode: e.ExitCode, Health: e.Health}
		if e.State != "running" || e.Health == "unhealthy" {
			logs, _ := p.run(ctx, "logs", "--no-color", "--no-log-prefix", "--tail", strconv.Itoa(logTailLines), s)
			sr.Logs = splitLines(logs)
		}
		res = append(res, sr)
	}
	return res
}

// parsePS reads docker compose ps JSON, which is one object per line on
// current releases and a single array on older ones.
func parsePS(out string) []psEntry {
	out = strings.TrimSpace(out)
	var all []psEntry
	if strings.HasPrefix(out, "[") {
		_ = json.Unmarshal([]byte(out), &all)
		return all
	}
	for _, line := range splitLines(out) {
		var e psEntry
		if json.Unmarshal([]byte(line), &e) == nil {
			all = append(all, e)
		}
	}
	return all
}

func docker(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // dev script: fixed binary, args built from catalog data
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if i := strings.LastIndex(msg, "\n"); i >= 0 {
			msg = strings.TrimSpace(msg[i+1:])
		}
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), fmt.Errorf("%s", msg)
	}
	return stdout.String(), nil
}

func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if l = strings.TrimRight(l, "\r"); strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}
