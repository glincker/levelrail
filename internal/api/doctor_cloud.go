package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Cloud preflight checks are read-only, optional and informational. They
// print commands for the operator; nothing here changes firewall state.
const (
	envDoctorCloudChecks          = "APP_DOCTOR_CLOUD_CHECKS"
	envDoctorCloudMetadata        = "APP_DOCTOR_CLOUD_METADATA"
	envDoctorCloudMetadataTimeout = "APP_DOCTOR_CLOUD_METADATA_TIMEOUT"
	envDoctorOracleIdlePercent    = "APP_DOCTOR_ORACLE_IDLE_PERCENT"
	envDoctorOracleIdleDays       = "APP_DOCTOR_ORACLE_IDLE_DAYS"

	// Defaults mirror the figures Oracle currently documents; both are env-tunable because the policy can change.
	defaultOracleIdlePercent = 20
	defaultOracleIdleDays    = 7

	oracleMetadataURL   = "http://169.254.169.254/opc/v2/instance/"
	oracleFreeTierDocs  = "https://docs.oracle.com/en-us/iaas/Content/FreeTier/resourceref.htm"
	doctorCloudDocsPath = "/host-firewall#iptables-based-hosts"
)

type cloudProviderInfo struct {
	ID, Name, Layer string
}

var cloudProviders = map[string]cloudProviderInfo{
	"oracle":       {"oracle", "Oracle Cloud Infrastructure", "a VCN security list or network security group"},
	"aws":          {"aws", "Amazon Web Services", "an EC2 security group and any network ACL"},
	"gcp":          {"gcp", "Google Cloud", "a VPC firewall rule"},
	"azure":        {"azure", "Microsoft Azure", "a network security group"},
	"hetzner":      {"hetzner", "Hetzner Cloud", "a Cloud Firewall"},
	"digitalocean": {"digitalocean", "DigitalOcean", "a Cloud Firewall"},
}

type cloudPort struct {
	Port int
	// Loopback is true when every binding of the port is on 127.0.0.0/8 or ::1.
	Loopback  bool
	Proto     string
	Container string
	Image     string
}

// cloudDeps holds every outside-world seam so tests run without files, commands or network.
type cloudDeps struct {
	getenv    func(string) string
	readFile  func(string) ([]byte, error)
	lookPath  func(string) (string, error)
	run       firewallCommandRunner
	metadata  func(ctx context.Context, url string) ([]byte, error)
	ports     func(ctx context.Context) ([]cloudPort, error)
	imageArch func(ctx context.Context, image string) (string, error)
	// failedImages lists image refs of apps whose latest deploy failed and so
	// have no running container for the port audit to see.
	failedImages func(ctx context.Context) []string
	// platformRefusals lists apps whose stored Ready condition is an architecture
	// refusal. A refused create never records a deploy attempt or leaves an image check to run.
	platformRefusals func(ctx context.Context) []string
	hostArch         string
}

func realCloudDeps() cloudDeps {
	return cloudDeps{
		getenv:   os.Getenv,
		readFile: os.ReadFile,
		lookPath: lookPathReal,
		run:      runRealFirewallCommand,
		metadata: realCloudMetadata,
	}
}

func realCloudMetadata(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer Oracle")
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metadata status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<16))
}

// doctorCheckCloud is the registered entry point. Ports come from the
// existing exposure audit so there is one source of truth for published ports.
func (rt *Router) doctorCheckCloud(ctx context.Context) []doctorCheckResource {
	d := realCloudDeps()
	if insp, ok := rt.dockerPinger.(docker.ImageDetailInspector); ok {
		if p, err := insp.NodePlatform(ctx); err == nil {
			d.hostArch = p.Arch
		}
		d.imageArch = func(ctx context.Context, image string) (string, error) {
			det, err := insp.InspectImageDetails(ctx, image)
			if err != nil {
				return "", err
			}
			return det.Arch, nil
		}
	}
	d.failedImages = rt.failedDeployImages
	d.platformRefusals = rt.platformRefusedApps
	d.ports = func(ctx context.Context) ([]cloudPort, error) {
		if rt.exposure == nil || rt.apps == nil || rt.databases == nil {
			return nil, nil
		}
		rep, err := rt.exposureReport(ctx, "", false)
		if err != nil {
			return nil, err
		}
		var out []cloudPort
		for _, n := range rep.Nodes {
			if !n.Local || n.Status != exposureNodeOK {
				continue
			}
			for _, f := range n.Findings {
				out = append(out, cloudPort{Port: f.HostPort, Proto: f.Protocol, Container: f.Container, Image: f.Image, Loopback: cloudBindsLoopback(f.Binds)})
			}
		}
		return append(out, rt.desiredCloudPortsNow(ctx)...), nil
	}
	return runCloudChecks(ctx, d)
}

// failedDeployImages returns the image of each app's newest attempt when it
// failed, so a failed create still gets an architecture check.
func (rt *Router) failedDeployImages(ctx context.Context) []string {
	if rt.apps == nil || rt.deployAttempts == nil {
		return nil
	}
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(services))
	for _, s := range services {
		names = append(names, s.Name)
	}
	return latestFailedImages(ctx, names, rt.deployAttempts.ListDeployAttempts)
}

// platformRefusedApps reads the stored Ready condition of each app, the same
// source app status shows, and keeps the architecture refusals.
func (rt *Router) platformRefusedApps(ctx context.Context) []string {
	if rt.apps == nil || rt.deploys == nil {
		return nil
	}
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil
	}
	controllers := make([]string, len(services))
	for i, s := range services {
		controllers[i] = applicationControllerName(s.Name)
	}
	conds, err := rt.deploys.GetConditionsForControllers(ctx, controllers)
	if err != nil {
		return nil
	}
	var out []string
	for _, s := range services {
		if msg := platformRefusalMessage(conds[applicationControllerName(s.Name)]); msg != "" {
			out = append(out, s.Name+" was refused: "+msg)
		}
	}
	return out
}

// platformRefusalMessage returns the refusal text of a failed Ready condition,
// or "" when the app is not stuck on an architecture mismatch.
func platformRefusalMessage(conds []reconcile.Condition) string {
	const marker = "PlatformMismatch"
	for _, c := range conds {
		if c.Status != reconcile.ConditionFalse {
			continue
		}
		if i := strings.Index(c.Message, marker+": "); i >= 0 {
			return strings.TrimSpace(c.Message[i+len(marker)+2:])
		}
		if c.Reason == marker {
			return strings.TrimSpace(c.Message)
		}
	}
	return ""
}

// latestFailedImages relies on the list being newest first, like the store.
func latestFailedImages(ctx context.Context, names []string, list func(context.Context, string) ([]store.DeployAttempt, error)) []string {
	var out []string
	for _, n := range names {
		attempts, err := list(ctx, n)
		if err != nil || len(attempts) == 0 {
			continue
		}
		if a := attempts[0]; a.Status == store.DeployAttemptStatusFailed && a.Image != "" {
			out = append(out, a.Image)
		}
	}
	return out
}

func (rt *Router) desiredCloudPortsNow(ctx context.Context) []cloudPort {
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil
	}
	var streams []store.AppStream
	if rt.appStreams != nil {
		streams, _ = rt.appStreams.ListAllAppStreams(ctx)
	}
	return desiredCloudPorts(services, streams)
}

func runCloudChecks(ctx context.Context, d cloudDeps) []doctorCheckResource {
	if strings.EqualFold(strings.TrimSpace(d.getenv(envDoctorCloudChecks)), "off") {
		return nil
	}
	prov, shape := detectCloudProvider(ctx, d)
	var ports []cloudPort
	if d.ports != nil {
		pctx, cancel := context.WithTimeout(ctx, doctorPingTimeout*2)
		ports, _ = d.ports(pctx)
		cancel()
	}
	ports = cloudDedupPorts(ports)

	var out []doctorCheckResource
	if prov != nil {
		out = append(out, cloudProviderResult(*prov, shape), cloudLayersResult(*prov))
	}
	if r, ok := hostFirewallResult(ctx, d, ports, prov != nil); ok {
		out = append(out, r)
	}
	if r, ok := publishedPortsResult(ports, prov); ok {
		out = append(out, r)
	}
	if r, ok := imageArchResult(ctx, d, ports); ok {
		out = append(out, r)
	}
	if r, ok := idleReclaimResult(d, prov, shape); ok {
		out = append(out, r)
	}
	return out
}

// detectCloudProvider reads local DMI files first. The link-local metadata
// call is opt-in and short, and only ever used to learn the Oracle shape.
func detectCloudProvider(ctx context.Context, d cloudDeps) (*cloudProviderInfo, string) {
	var sb strings.Builder
	for _, f := range []string{"sys_vendor", "product_name", "chassis_asset_tag", "bios_vendor"} {
		if b, err := d.readFile("/sys/class/dmi/id/" + f); err == nil {
			sb.WriteString(strings.ToLower(strings.TrimSpace(string(b))))
			sb.WriteByte('\n')
		}
	}
	dmi := sb.String()
	id := ""
	switch {
	case strings.Contains(dmi, "oraclecloud.com"):
		id = "oracle"
	case strings.Contains(dmi, "amazon ec2"), strings.Contains(dmi, "amazon\n"):
		id = "aws"
	case strings.Contains(dmi, "google compute engine"):
		id = "gcp"
	case strings.Contains(dmi, "7783-7084-3265-9085-8269-3286-77"):
		id = "azure"
	case strings.Contains(dmi, "hetzner"):
		id = "hetzner"
	case strings.Contains(dmi, "digitalocean"):
		id = "digitalocean"
	}
	metaOn := strings.EqualFold(strings.TrimSpace(d.getenv(envDoctorCloudMetadata)), "on")
	if (id == "" || id == "oracle") && metaOn && d.metadata != nil {
		to := envDurationOr(envDoctorCloudMetadataTimeout, 300*time.Millisecond)
		mctx, cancel := context.WithTimeout(ctx, to)
		defer cancel()
		if b, err := d.metadata(mctx, oracleMetadataURL); err == nil {
			var m struct {
				Shape string `json:"shape"`
			}
			if json.Unmarshal(b, &m) == nil && m.Shape != "" {
				p := cloudProviders["oracle"]
				return &p, m.Shape
			}
		}
	}
	if id == "" {
		return nil, ""
	}
	p := cloudProviders[id]
	return &p, ""
}

func cloudProviderResult(p cloudProviderInfo, shape string) doctorCheckResource {
	msg := "Detected " + p.Name + " from local hardware identifiers. This is informational and never required."
	if shape != "" {
		msg = "Detected " + p.Name + ", shape " + shape + ". This is informational and never required."
	}
	return doctorCheckResource{Code: "cloud_provider", Name: "Cloud provider", Status: doctorStatusOK, Message: msg}
}

func cloudLayersResult(p cloudProviderInfo) doctorCheckResource {
	return doctorCheckResource{
		Code: "cloud_firewall_layers", Name: "Firewall layers", Status: doctorStatusOK,
		Message: "A port must be open on two separate layers: " + p.Layer + " on the " + p.Name +
			" side, and the firewall on this host. Levelrail cannot see or change the provider side, and opening one layer does not open the other.",
		DocsPath: doctorCloudDocsPath,
	}
}

// hostFirewallResult reports the iptables backend only when it is the one in use,
// so hosts on ufw or firewalld, or with no readable iptables, see no change.
func hostFirewallResult(ctx context.Context, d cloudDeps, ports []cloudPort, cloud bool) (doctorCheckResource, bool) {
	const code, name = "host_firewall_iptables", "Host firewall (iptables)"
	fctx, cancel := context.WithTimeout(ctx, doctorPingTimeout)
	defer cancel()

	if _, err := d.lookPath("iptables"); err != nil {
		return doctorCheckResource{}, false
	}
	if _, err := d.lookPath("ufw"); err == nil {
		if out, err := d.run(fctx, "ufw", "status"); err == nil && strings.Contains(string(out), "Status: active") {
			return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "ufw is the active host firewall; see the ufw rules managed by Levelrail"}, true
		}
	}
	if _, err := d.lookPath("firewall-cmd"); err == nil {
		if out, err := d.run(fctx, "firewall-cmd", "--state"); err == nil && strings.TrimSpace(string(out)) == "running" {
			return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "firewalld is the active host firewall"}, true
		}
	}
	out, err := d.run(fctx, "iptables", "-S", "INPUT")
	if err != nil {
		if !cloud {
			return doctorCheckResource{}, false
		}
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "iptables -S INPUT failed, may need elevated privileges"}, true
	}
	pos, accepts := cloudAnalyzeInput(string(out))
	if pos == 0 {
		return doctorCheckResource{}, false
	}
	persist := cloudPersistCommand(d)
	cmds := cloudOpenCommands(ports, pos, persist)
	if accepts {
		return doctorCheckResource{
			Code: code, Name: name, Status: doctorStatusWarn,
			Message: fmt.Sprintf("INPUT has a catch-all REJECT/DROP at position %d and ACCEPT rules after it. Rules appended with -A land after it and are never reached; insert before it with -I", pos),
			Fix:     cmds, DocsPath: doctorCloudDocsPath,
		}, true
	}
	return doctorCheckResource{
		Code: code, Name: name, Status: doctorStatusOK,
		Message: fmt.Sprintf("iptables INPUT ends in a catch-all REJECT/DROP at position %d. To open a host port, insert before it (never append). Commands are printed only, nothing was applied", pos),
		Fix:     cmds, DocsPath: doctorCloudDocsPath,
	}, true
}

// cloudAnalyzeInput finds the 1-based position of the first unconditional
// REJECT/DROP in INPUT and whether any ACCEPT rule follows it.
func cloudAnalyzeInput(s string) (pos int, acceptAfter bool) {
	n := 0
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "-A INPUT") {
			continue
		}
		n++
		f := strings.Fields(line)
		target := ""
		for i, w := range f {
			if w == "-j" && i+1 < len(f) {
				target = f[i+1]
			}
		}
		if pos == 0 && (target == "REJECT" || target == "DROP") && cloudUnconditional(f) {
			pos = n
			continue
		}
		if pos != 0 && target == "ACCEPT" {
			acceptAfter = true
		}
	}
	return pos, acceptAfter
}

// cloudUnconditional is true when the rule has no match options, only the target and its arguments.
func cloudUnconditional(f []string) bool { return len(f) > 3 && f[2] == "-j" }

func cloudPersistCommand(d cloudDeps) string {
	if _, err := d.lookPath("netfilter-persistent"); err == nil {
		return "sudo netfilter-persistent save"
	}
	return "sudo sh -c 'iptables-save > /etc/iptables/rules.v4'"
}

func cloudSafeProto(p string) bool { return p == "tcp" || p == "udp" }

func cloudOpenCommands(ports []cloudPort, pos int, persist string) string {
	var lines []string
	for _, p := range ports {
		if p.Loopback || p.Port < 1 || p.Port > 65535 || !cloudSafeProto(p.Proto) {
			continue
		}
		lines = append(lines, fmt.Sprintf("sudo iptables -I INPUT %d -p %s --dport %d -j ACCEPT", pos, p.Proto, p.Port))
	}
	if len(lines) == 0 {
		lines = []string{fmt.Sprintf("sudo iptables -I INPUT %d -p <tcp|udp> --dport <PORT> -j ACCEPT", pos)}
	}
	return strings.Join(append(lines, persist), "\n")
}

func cloudDedupPorts(in []cloudPort) []cloudPort {
	seen := map[string]int{}
	var out []cloudPort
	for _, p := range in {
		if p.Port < 1 || p.Port > 65535 || !cloudSafeProto(p.Proto) {
			continue
		}
		k := p.Proto + ":" + strconv.Itoa(p.Port)
		if i, ok := seen[k]; ok {
			// A port is private only when every binding of it is.
			out[i].Loopback = out[i].Loopback && p.Loopback
			continue
		}
		seen[k] = len(out)
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Port != out[j].Port {
			return out[i].Port < out[j].Port
		}
		return out[i].Proto < out[j].Proto
	})
	return out
}

func publishedPortsResult(ports []cloudPort, prov *cloudProviderInfo) (doctorCheckResource, bool) {
	var open, private []string
	for _, p := range ports {
		label := fmt.Sprintf("%d/%s", p.Port, p.Proto)
		if p.Loopback {
			private = append(private, label)
		} else {
			open = append(open, label)
		}
	}
	if len(open) == 0 && len(private) == 0 {
		return doctorCheckResource{}, false
	}
	if len(open) == 0 {
		return doctorCheckResource{
			Code: "published_port_reachability", Name: "Published port reachability", Status: doctorStatusOK,
			Message:  "Published ports are bound to loopback only (" + strings.Join(private, ", ") + "), so nothing needs opening on the firewall. They are reachable from this host or an SSH tunnel.",
			DocsPath: doctorCloudDocsPath,
		}, true
	}
	where := "your network edge (router or cloud firewall) and this host's firewall"
	if prov != nil {
		where = prov.Layer + " on " + prov.Name + ", and this host's firewall"
	}
	return doctorCheckResource{
		Code: "published_port_reachability", Name: "Published port reachability", Status: doctorStatusOK,
		Message: "Published ports: " + strings.Join(open, ", ") + ". Each must be open on " + where +
			". This check does not probe from outside, so it cannot confirm they are reachable." + privateNote(private),
		DocsPath: doctorCloudDocsPath,
	}, true
}

// cloudBindsLoopback is true only when every bind is a loopback IP. No binds,
// a wildcard or an unparseable address counts as reachable, the safe side.
func cloudBindsLoopback(binds []string) bool {
	if len(binds) == 0 {
		return false
	}
	for _, b := range binds {
		ip := net.ParseIP(strings.Trim(strings.TrimSpace(b), "[]"))
		if ip == nil || !ip.IsLoopback() {
			return false
		}
	}
	return true
}

func privateNote(private []string) string {
	if len(private) == 0 {
		return ""
	}
	return " Loopback-only ports need no firewall change: " + strings.Join(private, ", ") + "."
}

func cloudSafeImage(s string) bool {
	return s != "" && !strings.HasPrefix(s, "-") && !strings.ContainsAny(s, " \t\r\n")
}

func imageArchResult(ctx context.Context, d cloudDeps, ports []cloudPort) (doctorCheckResource, bool) {
	var bad []string
	if d.platformRefusals != nil {
		bad = append(bad, d.platformRefusals(ctx)...)
	}
	images := make([]string, 0, len(ports))
	for _, p := range ports {
		images = append(images, p.Image)
	}
	if d.failedImages != nil {
		images = append(images, d.failedImages(ctx)...)
	}
	if d.imageArch == nil || d.hostArch == "" {
		images = nil
	}
	seen := map[string]bool{}
	for _, img := range images {
		if seen[img] || !cloudSafeImage(img) {
			continue
		}
		seen[img] = true
		ictx, cancel := context.WithTimeout(ctx, doctorPingTimeout)
		arch, err := d.imageArch(ictx, img)
		cancel()
		if err != nil || arch == "" || arch == d.hostArch {
			continue
		}
		bad = append(bad, fmt.Sprintf("%s is %s", img, arch))
	}
	if len(bad) == 0 {
		return doctorCheckResource{}, false
	}
	sort.Strings(bad)
	return doctorCheckResource{
		Code: "image_arch", Name: "Image architecture", Status: doctorStatusWarn,
		Message: "Local images do not match this node (" + d.hostArch + "): " + strings.Join(bad, "; ") +
			". Without emulation on the node, a pull or start fails with PlatformMismatch in the deploy trace; " +
			"with qemu binfmt emulation present it runs under emulation and may be slow.",
		Fix: "docker buildx imagetools inspect <image>  # look for a linux/" + d.hostArch + " manifest, or pick another tag",
	}, true
}

func idleReclaimResult(d cloudDeps, prov *cloudProviderInfo, shape string) (doctorCheckResource, bool) {
	if prov == nil || prov.ID != "oracle" {
		return doctorCheckResource{}, false
	}
	if shape != "" && shape != "VM.Standard.E2.1.Micro" && shape != "VM.Standard.A1.Flex" {
		return doctorCheckResource{}, false
	}
	pct := envInt64FromGetenv(d.getenv, envDoctorOracleIdlePercent, defaultOracleIdlePercent)
	days := envInt64FromGetenv(d.getenv, envDoctorOracleIdleDays, defaultOracleIdleDays)
	return doctorCheckResource{
		Code: "idle_reclaim_risk", Name: "Idle reclaim risk", Status: doctorStatusOK,
		Message: fmt.Sprintf("If this is an Always Free instance, Oracle has stated it may reclaim instances that stay idle (CPU, network and memory use all below about %d percent over %d days). "+
			"Oracle's policy can change; read the current terms at %s. Do not use sleep-when-idle on such a host, it makes the instance look more idle. Levelrail does not generate artificial load.",
			pct, days, oracleFreeTierDocs),
	}, true
}

func envInt64FromGetenv(get func(string) string, key string, def int64) int64 {
	if n, err := strconv.ParseInt(strings.TrimSpace(get(key)), 10, 64); err == nil && n > 0 {
		return n
	}
	return def
}
