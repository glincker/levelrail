package iac

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

type probeFields struct {
	Path            string   `json:"path,omitempty" yaml:"path,omitempty"`
	Scheme          string   `json:"scheme,omitempty" yaml:"scheme,omitempty"`
	Host            string   `json:"host,omitempty" yaml:"host,omitempty"`
	TLSSkipVerify   bool     `json:"tls_skip_verify,omitempty" yaml:"tls_skip_verify,omitempty"`
	FollowRedirects *bool    `json:"follow_redirects,omitempty" yaml:"follow_redirects,omitempty"`
	ExpectedStatus  string   `json:"expected_status,omitempty" yaml:"expected_status,omitempty"`
	Exec            []string `json:"exec,omitempty" yaml:"exec,omitempty"`
	Interval        string   `json:"interval,omitempty" yaml:"interval,omitempty"`
	Timeout         string   `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	Failures        int      `json:"failures,omitempty" yaml:"failures,omitempty"`
}

type healthFields struct {
	Readiness    *probeFields `json:"readiness,omitempty" yaml:"readiness,omitempty"`
	Liveness     *probeFields `json:"liveness,omitempty" yaml:"liveness,omitempty"`
	ReadyTimeout string       `json:"readyTimeout,omitempty" yaml:"readyTimeout,omitempty"`
}

type resourceFields struct {
	Memory     string  `json:"memory,omitempty" yaml:"memory,omitempty"`
	CPU        float64 `json:"cpu,omitempty" yaml:"cpu,omitempty"`
	SwapMemory string  `json:"swapMemory,omitempty" yaml:"swapMemory,omitempty"`
	CPUSet     string  `json:"cpuSet,omitempty" yaml:"cpuSet,omitempty"`
}

type hookFields struct {
	PreDeploy  string `json:"preDeploy,omitempty" yaml:"preDeploy,omitempty"`
	PostDeploy string `json:"postDeploy,omitempty" yaml:"postDeploy,omitempty"`
}

// appFields is the canonical, comparable shape of an app on both the
// desired and the live side.
type appFields struct {
	Image       string            `json:"image"`
	Port        int               `json:"port"`
	HostPort    int               `json:"hostPort,omitempty"`
	BindAddress string            `json:"bindAddress"`
	Replicas    int               `json:"replicas"`
	Strategy    string            `json:"strategy"`
	Domains     []string          `json:"domains,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	SecretEnv   []string          `json:"secretEnv,omitempty"`
	Resources   *resourceFields   `json:"resources,omitempty"`
	Health      *healthFields     `json:"health,omitempty"`
	Hooks       *hookFields       `json:"hooks,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Command     []string          `json:"command,omitempty"`
	Project     string            `json:"project,omitempty"`
	Environment string            `json:"environment,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
}

var additiveApp = map[string]bool{"domains": true, "env": true, "secretEnv": true, "labels": true, "tags": true}

func memoryString(b int64) string {
	const mi = 1024 * 1024
	if b%(1024*mi) == 0 {
		return strconv.FormatInt(b/(1024*mi), 10) + "Gi"
	}
	return strconv.FormatInt(b/mi, 10) + "Mi"
}

func parseMemory(s string) (int64, error) {
	switch {
	case strings.HasSuffix(s, "Gi"):
		n, err := strconv.ParseInt(strings.TrimSuffix(s, "Gi"), 10, 64)
		return n << 30, err
	case strings.HasSuffix(s, "Mi"):
		n, err := strconv.ParseInt(strings.TrimSuffix(s, "Mi"), 10, 64)
		return n << 20, err
	}
	return 0, fmt.Errorf("invalid memory %q", s)
}

func probeFromSpec(p *spec.Probe) (*probeFields, error) {
	if p == nil {
		return nil, nil
	}
	iv, err := normDuration(p.Interval)
	if err != nil {
		return nil, err
	}
	to, err := normDuration(p.Timeout)
	if err != nil {
		return nil, err
	}
	return &probeFields{Path: p.Path, Scheme: p.Scheme, Host: p.Host, TLSSkipVerify: p.TLSSkipVerify, FollowRedirects: p.FollowRedirects,
		ExpectedStatus: string(p.ExpectedStatus), Exec: []string(p.Exec), Interval: iv, Timeout: to, Failures: p.Failures}, nil
}

func healthFromSpec(h *spec.Health) (*healthFields, error) {
	if h == nil {
		return nil, nil
	}
	var out healthFields
	var err error
	if out.Readiness, err = probeFromSpec(h.Readiness); err != nil {
		return nil, fmt.Errorf("readiness: %w", err)
	}
	if out.Liveness, err = probeFromSpec(h.Liveness); err != nil {
		return nil, fmt.Errorf("liveness: %w", err)
	}
	if out.ReadyTimeout, err = normDuration(h.ReadyTimeout); err != nil {
		return nil, fmt.Errorf("readyTimeout: %w", err)
	}
	return nonEmptyHealth(&out), nil
}

func nonEmptyHealth(h *healthFields) *healthFields {
	if h == nil || (h.Readiness == nil && h.Liveness == nil && h.ReadyTimeout == "") {
		return nil
	}
	return h
}

func probeFromStore(p *store.ServiceProbe) *probeFields {
	if p == nil {
		return nil
	}
	return &probeFields{Path: p.Path, Scheme: p.Scheme, Host: p.Host, TLSSkipVerify: p.TLSSkipVerify, FollowRedirects: p.FollowRedirects,
		ExpectedStatus: p.ExpectedStatus, Exec: p.Exec, Interval: fmtDuration(p.Interval), Timeout: fmtDuration(p.Timeout), Failures: p.Failures}
}

func healthFromStore(h *store.ServiceHealth) *healthFields {
	if h == nil {
		return nil
	}
	return nonEmptyHealth(&healthFields{Readiness: probeFromStore(h.Readiness), Liveness: probeFromStore(h.Liveness), ReadyTimeout: fmtDuration(h.ReadyTimeout)})
}

func resourcesFromSpec(r *spec.Resources) *resourceFields {
	if r == nil {
		return nil
	}
	out := resourceFields{Memory: r.Memory, CPU: r.CPU, SwapMemory: r.SwapMemory, CPUSet: r.CPUSet}
	if out == (resourceFields{}) {
		return nil
	}
	return &out
}

func resourcesFromStore(r *store.ServiceResources) *resourceFields {
	if r == nil {
		return nil
	}
	out := resourceFields{CPUSet: r.CPUSetCPUs}
	if r.MemoryBytes > 0 {
		out.Memory = memoryString(r.MemoryBytes)
	}
	if r.SwapMemoryBytes > 0 {
		out.SwapMemory = memoryString(r.SwapMemoryBytes)
	}
	if r.NanoCPUs > 0 {
		out.CPU = float64(r.NanoCPUs) / 1e9
	}
	if out == (resourceFields{}) {
		return nil
	}
	return &out
}

func probeToStore(p *probeFields) *store.ServiceProbe {
	if p == nil {
		return nil
	}
	iv, _ := time.ParseDuration(p.Interval)
	to, _ := time.ParseDuration(p.Timeout)
	return &store.ServiceProbe{Path: p.Path, Scheme: p.Scheme, Host: p.Host, TLSSkipVerify: p.TLSSkipVerify, FollowRedirects: p.FollowRedirects,
		ExpectedStatus: p.ExpectedStatus, Exec: p.Exec, Interval: iv, Timeout: to, Failures: p.Failures}
}

func healthToStore(h *healthFields) *store.ServiceHealth {
	if h == nil {
		return nil
	}
	rt, _ := time.ParseDuration(h.ReadyTimeout)
	return &store.ServiceHealth{Readiness: probeToStore(h.Readiness), Liveness: probeToStore(h.Liveness), ReadyTimeout: rt}
}

func resourcesToStore(r *resourceFields) *store.ServiceResources {
	if r == nil {
		return nil
	}
	out := store.ServiceResources{CPUSetCPUs: r.CPUSet, NanoCPUs: int64(r.CPU * 1e9)}
	if r.Memory != "" {
		out.MemoryBytes, _ = parseMemory(r.Memory)
	}
	if r.SwapMemory != "" {
		out.SwapMemoryBytes, _ = parseMemory(r.SwapMemory)
	}
	return &out
}

// appFromWire builds the canonical live shape; names resolves ids to names.
func appFromWire(a wireApp, names idNames) appFields {
	f := appFields{
		Image: a.Image, Port: a.Port, BindAddress: a.BindAddress, Replicas: a.Replicas, Strategy: a.Strategy,
		Domains: sortedUnique(a.Domains), Env: nonEmptyMap(a.Env), SecretEnv: sortedUnique(a.SecretEnv),
		Resources: resourcesFromStore(a.Resources), Health: healthFromStore(a.Health), Labels: nonEmptyMap(a.Labels),
		Command: a.Command, Project: names.project(a.ProjectID), Environment: names.environment(a.EnvironmentID), Tags: sortedUnique(a.Tags),
	}
	if a.HostPort != nil {
		f.HostPort = *a.HostPort
	}
	if a.Hooks != nil && (a.Hooks.PreDeploy != "" || a.Hooks.PostDeploy != "") {
		f.Hooks = &hookFields{PreDeploy: a.Hooks.PreDeploy, PostDeploy: a.Hooks.PostDeploy}
	}
	return f
}

func nonEmptyMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	return m
}

// unsupportedApp lists live settings apply cannot express, so an export
// or update can warn instead of silently dropping them.
func unsupportedApp(a wireApp) []string {
	var out []string
	if len(a.Volumes) > 0 {
		out = append(out, "volumes")
	}
	if len(a.BindMounts) > 0 {
		out = append(out, "bind mounts")
	}
	if len(a.Egress) > 0 {
		out = append(out, "egress policy")
	}
	if len(a.VaultEnv) > 0 {
		out = append(out, "vault env")
	}
	return out
}
