package catalog

import (
	"fmt"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/compose"
)

// RequirementsLevel summarizes how much trust a template asks of the host.
type RequirementsLevel string

// Requirements levels, ordered standard < review < elevated.
const (
	LevelStandard RequirementsLevel = "standard"
	LevelReview   RequirementsLevel = "review"
	LevelElevated RequirementsLevel = "elevated"
)

// Finding severities, ordered info < review < elevated.
const (
	SeverityInfo     = "info"
	SeverityReview   = "review"
	SeverityElevated = "elevated"
)

// Finding codes, stable identifiers the dashboard maps to translated copy.
const (
	FindingPrivileged       = "privileged"
	FindingDockerSocket     = "docker_socket"
	FindingHostNetwork      = "host_network"
	FindingHostNamespace    = "host_namespace"
	FindingCapAdd           = "cap_add"
	FindingDevices          = "devices"
	FindingSecurityOpt      = "security_opt"
	FindingHostMount        = "host_mount"
	FindingUnpinnedImage    = "unpinned_image"
	FindingPublishedPorts   = "published_ports"
	FindingGeneratedSecrets = "generated_secrets"
	FindingRequiredSecrets  = "required_secrets"
	FindingNoHealthcheck    = "no_healthcheck"
	FindingPersistentData   = "persistent_data"
	FindingGPU              = "gpu"
	FindingUnparsed         = "unparsed"
)

// Protection codes for what Levelrail adds around a template.
const (
	ProtectionIngress  = "https_ingress"
	ProtectionMasking  = "secret_masking"
	ProtectionBackups  = "volume_backups"
	ProtectionHealth   = "health_gated_deploy"
	ProtectionRollback = "rollback"
	ProtectionExposure = "exposure_report"
	ProtectionFirewall = "firewall"
)

// Finding is one thing a template asks of the host or the operator.
type Finding struct {
	Code     string   `json:"code"`
	Severity string   `json:"severity"`
	Services []string `json:"services,omitempty"`
	Detail   string   `json:"detail,omitempty"`
	Message  string   `json:"message"`
}

// Protection is one Levelrail safeguard that applies to a template.
type Protection struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

// Requirements is the computed "what this needs, what Levelrail adds" card.
type Requirements struct {
	Level       RequirementsLevel `json:"level"`
	Findings    []Finding         `json:"findings"`
	Protections []Protection      `json:"protections"`
}

var protectionLabels = map[string]string{
	ProtectionIngress:  "Automatic HTTPS and an ingress shield in front of every domain",
	ProtectionMasking:  "Secrets are generated, stored encrypted and masked in logs and the UI",
	ProtectionBackups:  "Named volumes can be backed up and restored",
	ProtectionHealth:   "Deploys are gated on the health check before traffic switches",
	ProtectionRollback: "One-step rollback to the previous revision",
	ProtectionExposure: "Exposure report shows what is reachable from outside",
	ProtectionFirewall: "Host firewall rules managed for published ports",
}

type findingSpec struct {
	code, severity, msg string
}

var findingSpecs = []findingSpec{
	{FindingPrivileged, SeverityElevated, "Runs in privileged mode, which gives the container near-root access to the host"},
	{FindingDockerSocket, SeverityElevated, "Mounts the Docker socket, which is equivalent to root on the host"},
	{FindingHostNetwork, SeverityElevated, "Uses host networking and bypasses the app network and ingress"},
	{FindingHostNamespace, SeverityElevated, "Shares a host process, IPC or user namespace"},
	{FindingCapAdd, SeverityElevated, "Adds extra Linux capabilities"},
	{FindingDevices, SeverityElevated, "Passes host devices into the container"},
	{FindingSecurityOpt, SeverityElevated, "Changes container security options"},
	{FindingHostMount, SeverityReview, "Mounts a directory from the host"},
	{FindingUnpinnedImage, SeverityReview, "Image is not pinned to a version tag, so a redeploy can pull different code"},
	{FindingPublishedPorts, SeverityInfo, "Publishes ports directly on the host"},
	{FindingRequiredSecrets, SeverityInfo, "You must provide one or more secrets before it can deploy"},
	{FindingGeneratedSecrets, SeverityInfo, "Passwords and keys are generated at deploy time"},
	{FindingNoHealthcheck, SeverityInfo, "No health check, so a deploy is judged by the process starting"},
	{FindingPersistentData, SeverityInfo, "Stores data in named volumes"},
	{FindingGPU, SeverityInfo, "Reserves an NVIDIA GPU"},
}

func noopGenerate(_, _ string, _ int) (string, error) { return "x", nil }
func noopPersist(_, _, _ string) error                { return nil }

// ComputeRequirements derives the requirements card from a template's
// Compose body alone. A body that fails to parse is reported as review.
func ComputeRequirements(composeBody string) Requirements {
	f, err := compose.Parse([]byte(composeBody))
	if err != nil {
		return Requirements{
			Level:       LevelReview,
			Findings:    []Finding{{Code: FindingUnparsed, Severity: SeverityReview, Message: "The Compose body could not be analyzed, review it before deploying"}},
			Protections: protections(false, false, false),
		}
	}

	by := map[string][]string{}
	details := map[string][]string{}
	note := func(code, svc, detail string) {
		by[code] = append(by[code], svc)
		if detail != "" {
			details[code] = append(details[code], detail)
		}
	}

	hasVolumes, hasHealth := false, false
	for name, svc := range f.Services {
		h := svc.HostAccess
		if h.Privileged {
			note(FindingPrivileged, name, "")
		}
		if strings.EqualFold(strings.TrimSpace(h.NetworkMode), "host") {
			note(FindingHostNetwork, name, "")
		}
		for _, ns := range []string{h.Pid, h.Ipc, h.Uts, h.UsernsMode, h.Cgroup} {
			if strings.EqualFold(strings.TrimSpace(ns), "host") {
				note(FindingHostNamespace, name, "")
				break
			}
		}
		if len(h.CapAdd) > 0 {
			note(FindingCapAdd, name, strings.Join(h.CapAdd, ", "))
		}
		if len(h.Devices) > 0 {
			note(FindingDevices, name, "")
		}
		if len(h.SecurityOpt) > 0 {
			note(FindingSecurityOpt, name, strings.Join(h.SecurityOpt, ", "))
		}
		for _, v := range svc.Volumes {
			switch {
			case v.HostPath != "" && strings.HasSuffix(strings.TrimRight(v.HostPath, "/"), "docker.sock"):
				note(FindingDockerSocket, name, v.HostPath)
			case v.HostPath != "":
				note(FindingHostMount, name, v.HostPath)
			case v.Name != "":
				hasVolumes = true
				note(FindingPersistentData, name, v.Name)
			}
		}
		if !pinned(svc.Image) {
			note(FindingUnpinnedImage, name, svc.Image)
		}
		for _, p := range svc.Ports {
			if p.HostPort > 0 {
				note(FindingPublishedPorts, name, fmt.Sprintf("%d:%d", p.HostPort, p.ContainerPort))
			}
		}
		if hasCheck(svc.Healthcheck) {
			hasHealth = true
		} else {
			note(FindingNoHealthcheck, name, "")
		}
		if svc.Deploy != nil && len(svc.Deploy.Resources.Reservations.Devices) > 0 {
			note(FindingGPU, name, "")
		}
	}

	hasSecrets := false
	if f2, err := compose.Parse([]byte(composeBody)); err == nil {
		if gen, unresolved, err := compose.ResolveMagicVars(f2, noopGenerate, noopPersist); err == nil {
			for svc, keys := range gen {
				if len(keys) > 0 {
					hasSecrets = true
					note(FindingGeneratedSecrets, svc, strings.Join(dedupe(keys), ", "))
				}
			}
			for _, u := range unresolved {
				hasSecrets = true
				note(FindingRequiredSecrets, u.Service, u.EnvKey)
			}
		}
	}

	level := LevelStandard
	findings := []Finding{}
	for _, s := range findingSpecs {
		svcs := dedupe(by[s.code])
		if len(svcs) == 0 {
			continue
		}
		findings = append(findings, Finding{Code: s.code, Severity: s.severity, Services: svcs, Detail: strings.Join(dedupe(details[s.code]), ", "), Message: s.msg})
		switch {
		case s.severity == SeverityElevated:
			level = LevelElevated
		case s.severity == SeverityReview && level == LevelStandard:
			level = LevelReview
		}
	}
	return Requirements{Level: level, Findings: findings, Protections: protections(hasSecrets, hasVolumes, hasHealth)}
}

func protections(secrets, volumes, health bool) []Protection {
	codes := []string{ProtectionIngress}
	if secrets {
		codes = append(codes, ProtectionMasking)
	}
	if volumes {
		codes = append(codes, ProtectionBackups)
	}
	if health {
		codes = append(codes, ProtectionHealth)
	}
	codes = append(codes, ProtectionRollback, ProtectionExposure, ProtectionFirewall)
	out := make([]Protection, 0, len(codes))
	for _, c := range codes {
		out = append(out, Protection{Code: c, Label: protectionLabels[c]})
	}
	return out
}

func pinned(image string) bool {
	if image == "" || strings.Contains(image, "${") || strings.Contains(image, "@sha256:") {
		return true
	}
	last := image[strings.LastIndex(image, "/")+1:]
	i := strings.LastIndex(last, ":")
	return i >= 0 && last[i+1:] != "latest"
}

func hasCheck(h *compose.Healthcheck) bool {
	return h != nil && len(h.Test) > 0 && !strings.EqualFold(h.Test[0], "NONE")
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
