package importplan

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/glincker/levelrail/internal/compose"
	"github.com/glincker/levelrail/internal/importreport"
)

var (
	quotedRe    = regexp.MustCompile(`"([^"]+)"`)
	keyPrefixRe = regexp.MustCompile(`(?:^|: )([a-z_][a-z0-9_.]*): `)
)

// buildReport fills the fidelity report for compose and docker run plans.
// It only reads the plan and source text and never changes what deploys.
func buildReport(p *DeploymentPlan, text string) {
	var out []importreport.Finding
	switch p.Source {
	case SourceCompose:
		out = composeReport(p, text)
	case SourceDockerRun:
		out = dockerRunReport(p)
	default:
		return
	}
	p.Report = importreport.Normalize(out)
}

func composeReport(p *DeploymentPlan, text string) []importreport.Finding {
	const kind = importreport.KindCompose
	var out []importreport.Finding
	for _, w := range p.Warnings {
		switch w.Code {
		case "compose_invalid":
			if strings.HasSuffix(w.Message, ":") {
				continue
			}
			msg := strings.TrimPrefix(w.Message, "- ")
			svc, key := validationPath(msg)
			out = append(out, importreport.Finding{Kind: kind, Path: svcPath(svc, key), Severity: importreport.SeverityBlocker,
				Reason: msg, Fix: "remove or change the field; the file cannot deploy as written"})
		case "compose_build":
			out = append(out, importreport.Finding{Kind: kind, Path: svcPath(firstQuoted(w.Message), "build"), Severity: importreport.SeverityBlocker,
				Reason: "a pasted file has no source to build from", Fix: "import the repository, or push the image and reference it with image:"})
		case "unpinned_tag":
			svc, _, _ := strings.Cut(w.Message, ": ")
			out = append(out, importreport.Finding{Kind: kind, Path: svcPath(svc, "image"), Severity: importreport.SeverityIgnoredSafe,
				Reason: "the image uses the latest tag, which can change between deploys", Fix: "pin a version or digest"})
		}
	}
	f, err := compose.Parse([]byte(text))
	if err != nil {
		return out
	}
	for _, u := range f.Unsupported {
		sev := importreport.SeverityIgnoredSafe
		if u.ChangesBehavior() {
			sev = importreport.SeverityChangesBehavior
		}
		out = append(out, importreport.Finding{Kind: kind, Path: svcPath(u.Service, u.Key), Severity: sev, Reason: u.Reason(), Fix: u.Alternative})
	}
	for _, n := range f.Notices() {
		if n.Key != "" || n.Level != compose.NoticeLevelWarning {
			continue
		}
		path := "networks"
		if svc := firstQuoted(n.Message); svc != "" {
			path = svcPath(svc, "restart")
		}
		out = append(out, importreport.Finding{Kind: kind, Path: path, Severity: importreport.SeverityChangesBehavior, Reason: n.Message,
			Fix: "rely on the shared app network, or split the stack into separate apps"})
	}
	for _, u := range p.Unsupported {
		out = append(out, importreport.Finding{Kind: kind, Path: svcPath(u.Service, u.Feature), Severity: unsupportedSeverity(u.Feature), Reason: u.Reason, Fix: u.Alternative})
	}
	return append(out, composeFieldFindings(f, text)...)
}

// unsupportedSeverity marks host access keys as blockers: Validate refuses them
// in a pasted file unless a manifest grants them.
func unsupportedSeverity(feature string) importreport.Severity {
	switch feature {
	case "privileged", "cap_add", "devices", "pid", "ipc", "security_opt", "network_mode", "build":
		return importreport.SeverityBlocker
	}
	return importreport.SeverityChangesBehavior
}

// composeFieldFindings covers fields that parse but behave differently here.
func composeFieldFindings(f *compose.File, text string) []importreport.Finding {
	const kind = importreport.KindCompose
	var out []importreport.Finding
	names := make([]string, 0, len(f.Services))
	for n := range f.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	nodes := serviceNodeRefs(text)
	for _, n := range names {
		s := f.Services[n]
		if s.Platform != "" {
			out = append(out, importreport.Finding{Kind: kind, Path: svcPath(n, "platform"), Severity: importreport.SeverityIgnoredSafe,
				Reason: fmt.Sprintf("platform %s is now pinned, so the service only runs on a node of that architecture", s.Platform),
				Fix:    "keep it if the image is single-arch, or remove it to let the node pick"})
		}
		if ref, ok := nodes[n]; ok {
			out = append(out, importreport.Finding{Kind: kind, Path: svcPath(n, "x-levelrail-node"), Severity: importreport.SeverityIgnoredSafe,
				Reason: fmt.Sprintf("the service is pinned to node %q; it stays pending if that node is missing", ref),
				Fix:    "make sure the node is enrolled before deploying"})
		}
		for _, pt := range s.Ports {
			if pt.HostPort == 0 {
				out = append(out, importreport.Finding{Kind: kind, Path: svcPath(n, fmt.Sprintf("ports.%d", pt.ContainerPort)), Severity: importreport.SeverityIgnoredSafe,
					Reason: "no host port is set, so the port is reachable through ingress and the app network only",
					Fix:    "add HOST:CONTAINER if it must be published on the node"})
			}
		}
		if hc := s.Healthcheck; hc != nil && len(hc.Test) > 0 && hc.Test[0] == "CMD-SHELL" {
			out = append(out, importreport.Finding{Kind: kind, Path: svcPath(n, "healthcheck.test"), Severity: importreport.SeverityChangesBehavior,
				Reason: "CMD-SHELL needs /bin/sh in the image; in a shell-less image the check always fails and the service reports unhealthy",
				Fix:    "use the CMD form with an executable the image contains"})
		}
		for _, v := range s.Volumes {
			if v.HostPath != "" {
				out = append(out, importreport.Finding{Kind: kind, Path: svcPath(n, "volumes."+v.ContainerPath), Severity: importreport.SeverityChangesBehavior,
					Reason: "a host bind mount needs root approval to create and ties the service to one node",
					Fix:    "use a named volume unless the data must live at that host path"})
			}
		}
	}
	return out
}

// serviceNodeRefs reads x-levelrail-node per service from the raw document.
func serviceNodeRefs(text string) map[string]string {
	var doc struct {
		Services map[string]map[string]any `yaml:"services"`
	}
	out := map[string]string{}
	if yaml.Unmarshal([]byte(text), &doc) != nil {
		return out
	}
	for n, svc := range doc.Services {
		if ref, ok := svc["x-levelrail-node"].(string); ok && ref != "" {
			out[n] = ref
		}
	}
	return out
}

func firstFields(s string, n int) string {
	f := strings.Fields(s)
	if len(f) < n {
		return s
	}
	return strings.Join(f[:n], " ")
}

func svcPath(svc, key string) string {
	if svc == "" {
		return key
	}
	return "services." + svc + "." + key
}

func firstQuoted(s string) string {
	if m := quotedRe.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

// validationPath best-effort reads the service and key out of a Validate line.
func validationPath(line string) (svc, key string) {
	svc = firstQuoted(line)
	if m := keyPrefixRe.FindStringSubmatch(line); m != nil {
		key = m[1]
	}
	if key == "" && strings.Contains(line, "bind-mount") {
		key = "volumes"
	}
	if key == "" {
		key = "validate"
	}
	return svc, key
}

// dockerFlagSeverity lists docker run flags whose loss changes how the
// container runs; every other unsupported flag is cosmetic or informational.
var dockerFlagSeverity = map[string]bool{
	"--privileged": true, "--cap-add": true, "--device": true, "--pid": true, "--ipc": true,
	"--uts": true, "--security-opt": true, "--userns": true, "--cgroupns": true, "--mount": true,
	"--link": true, "--volumes-from": true, "--runtime": true, "--gpus": true, "--user": true,
	"-u": true, "--workdir": true, "-w": true, "--health-cmd": true, "--network host": true,
	"--cpuset-cpus": true, "--pid host": true, "--memory-swap": true, "--stop-signal": true,
}

var dockerFlagFix = map[string]string{
	"--privileged":   "remove it, or grant only the specific capability the app needs in its manifest",
	"--cap-add":      "declare the capability as a grant in the manifest; ungranted capabilities are dropped",
	"--device":       "run on a node that exposes the device, then attach it after creating the app",
	"--network host": "publish the needed ports with -p; host networking needs a host_network grant",
	"--pid":          "remove it; host PID sharing is refused",
	"--gpus":         "set the GPU request on the app after creating it",
	"--health-cmd":   "add an HTTP health check after creating the app",
	"--platform":     "pin the platform on the app after creating it",
	"--mount":        "rewrite it as -v name:/path",
}

func dockerRunReport(p *DeploymentPlan) []importreport.Finding {
	const kind = importreport.KindDockerRun
	var out []importreport.Finding
	for _, w := range p.Warnings {
		path, sev, fix := "", importreport.SeverityChangesBehavior, ""
		switch w.Code {
		case "unsupported_flag":
			path, _, _ = strings.Cut(w.Message, ": ")
			if !dockerFlagSeverity[path] {
				sev = importreport.SeverityIgnoredSafe
			}
		case "env_from_host", "secret_reenter":
			path = firstFields(w.Message, 2)
		case "env_file":
			path = "--env-file"
		case "bind_mount":
			path = "-v " + strings.Fields(w.Message)[2]
		case "docker_socket":
			path = "-v docker.sock"
			fix = "remove it unless the app must control Docker; it gives the container control of the host"
		case "network":
			path = "--network"
		case "unpinned_tag":
			path, sev = "image", importreport.SeverityIgnoredSafe
		case "port_unknown":
			path = "-p"
		case "extra_ports":
			path = "-p (extra ports)"
		default:
			continue
		}
		reason := w.Message
		if w.Code == "unsupported_flag" {
			reason = strings.ReplaceAll(strings.TrimPrefix(w.Message, path+": "), " (not supported, ignored)", "")
		}
		if f, ok := dockerFlagFix[path]; ok {
			fix = f
		}
		if fix == "" && sev == importreport.SeverityIgnoredSafe {
			fix = "safe to leave out"
		} else if fix == "" {
			fix = "set it on the app after creating it, or leave it out"
		}
		out = append(out, importreport.Finding{Kind: kind, Path: path, Severity: sev, Reason: reason, Fix: fix})
	}
	for _, s := range p.Services {
		for _, pt := range s.Ports {
			if pt.Host == 0 {
				out = append(out, importreport.Finding{Kind: kind, Path: fmt.Sprintf("-p %d", pt.Container), Severity: importreport.SeverityIgnoredSafe,
					Reason: "no host port is set, so the port is reachable through ingress and the app network only",
					Fix:    "use -p HOST:CONTAINER if it must be published on the node"})
			}
		}
	}
	return out
}
