package application

import (
	"context"
	"fmt"
	"regexp"
	"sort"

	"github.com/GLINCKER/levelrail/internal/docker"
	appspec "github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// JobSpec is a one-off container spec derived from a service, plus the
// resolved values a job's captured output must never contain.
type JobSpec struct {
	Spec         docker.ContainerSpec
	SecretValues []string
}

// jobSecretKey flags env keys whose values are redacted from job output even
// when they did not come through the secret store.
var jobSecretKey = regexp.MustCompile(`(?i)(pass(word|wd)?|secret|token|api_?key|private_?key|credential|dsn|auth)`)

const minRedactedLen = 4

// BuildJobSpec returns the container spec for running command once in a
// fresh container of desired's image: the same resolved env, volumes,
// network, DNS, hosts and registry auth the service itself gets, without
// its published ports, labels or restart behavior. command is the exact
// argv (the image entrypoint is replaced), the same contract as exec mode.
func (c *Controller) BuildJobSpec(ctx context.Context, desired *store.DesiredService, name string, command []string, res *docker.Resources) (JobSpec, error) {
	if len(command) == 0 {
		return JobSpec{}, fmt.Errorf("build job spec: command is required")
	}
	env, err := c.resolveEnv(ctx, desired)
	if err != nil {
		return JobSpec{}, fmt.Errorf("resolve env: %w", err)
	}
	for _, v := range desired.Volumes {
		if err := docker.EnsureVolumeFor(ctx, c.runtime, v.Name, volumeOwner(desired, v.Name)); err != nil {
			return JobSpec{}, fmt.Errorf("ensure volume %q: %w", v.Name, err)
		}
	}
	spec, err := toContainerSpec(name, desired)
	if err != nil {
		return JobSpec{}, fmt.Errorf("container spec: %w", err)
	}
	spec.Ports = nil
	spec.GPU = nil
	spec.Labels = nil
	spec.Env = env
	spec.Entrypoint = []string{command[0]}
	spec.Command = append([]string(nil), command[1:]...)
	spec.ForcePull = false
	if c.instanceID != "" {
		spec.Labels = mergeLabel(spec.Labels, appspec.InstanceLabelKey, c.instanceID)
	}
	spec.Labels = mergeLabel(spec.Labels, appspec.AppLabelKey, volumeOwner(desired, "").App)
	if desired.Incarnation != "" {
		spec.Labels = mergeLabel(spec.Labels, appspec.IncarnationLabelKey, desired.Incarnation)
	}
	if res != nil {
		merged := docker.Resources{}
		if spec.Resources != nil {
			merged = *spec.Resources
		}
		if res.MemoryBytes > 0 {
			merged.MemoryBytes = res.MemoryBytes
		}
		if res.NanoCPUs > 0 {
			merged.NanoCPUs = res.NanoCPUs
		}
		spec.Resources = &merged
	}
	if desired.RegistryCredentialID != "" {
		auth, err := c.resolveRegistryAuth(ctx, desired.RegistryCredentialID)
		if err != nil {
			return JobSpec{}, fmt.Errorf("resolve registry credential: %w", err)
		}
		spec.RegistryAuth = auth
	}
	if c.meshDNSAddr != "" {
		spec.DNS = []string{c.meshDNSAddr}
	}
	if spec.ExtraHosts, err = c.databaseExtraHosts(ctx, desired); err != nil {
		return JobSpec{}, fmt.Errorf("database mesh hosts: %w", err)
	}
	if desired.AppID != "" {
		if err := c.ensureAppNetwork(ctx, desired); err != nil {
			return JobSpec{}, err
		}
		spec.Network = &docker.NetworkAttachment{Name: NetworkName(c.networkPrefix, desired.AppID)}
		if err := c.connectReferencedDatabases(ctx, desired, NetworkName(c.networkPrefix, desired.AppID)); err != nil {
			return JobSpec{}, err
		}
	}
	return JobSpec{Spec: spec, SecretValues: jobSecretValues(desired, env)}, nil
}

// JobSecretValues resolves the service's env and returns only the values to
// redact, for exec-mode runs that build no container spec.
func (c *Controller) JobSecretValues(ctx context.Context, desired *store.DesiredService) ([]string, error) {
	env, err := c.resolveEnv(ctx, desired)
	if err != nil {
		return nil, fmt.Errorf("resolve env: %w", err)
	}
	return jobSecretValues(desired, env), nil
}

func jobSecretValues(desired *store.DesiredService, env map[string]string) []string {
	keys := map[string]bool{}
	for _, s := range desired.SecretEnv {
		keys[s.Name] = true
	}
	for k := range desired.VaultEnv {
		keys[k] = true
	}
	for k := range desired.DatabaseEnv {
		keys[k] = true
	}
	seen := map[string]bool{}
	var out []string
	for k, v := range env {
		if len(v) < minRedactedLen || seen[v] {
			continue
		}
		if keys[k] || jobSecretKey.MatchString(k) {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}
