package api

import (
	"context"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/certflow"
	"github.com/GLINCKER/levelrail/internal/network"
	"github.com/GLINCKER/levelrail/internal/serverready"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/diskspace"
)

const (
	envDoctorInodeWarnPercent = "APP_DOCTOR_INODE_WARN_PERCENT"
	envDoctorOrphanWarnBytes  = "APP_DOCTOR_ORPHAN_WARN_BYTES"

	defaultDoctorInodeWarnPercent = 10.0
	defaultDoctorOrphanWarnBytes  = int64(2) << 30
	doctorDNSMaxDomains           = 50
	doctorDNSLookupTimeout        = 3 * time.Second
	doctorDNSParallelism          = 8
	doctorNodeSilentAfter         = 2 * time.Minute
)

func (rt *Router) doctorFromFix(ctx context.Context, code, name, fixID, okMsg string) doctorCheckResource {
	if rt.fixit.store == nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "the fix engine is not configured"}
	}
	v, err := rt.fixitEngine().Preview(ctx, fixID)
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("could not evaluate: %s", err)}
	}
	if !v.Failing {
		msg := okMsg
		if v.Detail != "" {
			msg = v.Detail
		}
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: msg}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn, Message: v.Detail, Evidence: v.Reason}
}

func (rt *Router) doctorCheckDashboardHTTPS(ctx context.Context) doctorCheckResource {
	return rt.doctorFromFix(ctx, "dashboard_url_https", "Dashboard over https", "settings.dashboard_url.https", "dashboard URL is fine")
}

func (rt *Router) doctorCheckBackupDrill(ctx context.Context) doctorCheckResource {
	return rt.doctorFromFix(ctx, "backup_drill", "Restore verification", "backup.drill.failing", "no failed restore drill")
}

func (rt *Router) doctorCheckPortCtx(ctx context.Context, port int) doctorCheckResource {
	c := rt.doctorCheckPort(port)
	if c.Status != doctorStatusFail {
		return c
	}
	if l, ok := serverready.HostTCPHolder(ctx, port); ok {
		holder := l.Process
		if l.Container != "" {
			holder = "container " + l.Container
		}
		if holder != "" {
			c.Message = "in use by " + holder
			c.Evidence = fmt.Sprintf("%s:%d held by %s (pid %d)", l.Addr, l.Port, holder, l.PID)
		}
	}
	c.Fix = "stop or reconfigure whatever holds port " + strconv.Itoa(port) + ", then re-check"
	c.DocsPath = "/domains-and-ingress#firewall-ports-80-and-443"
	return c
}

func (rt *Router) doctorCheckInodes(context.Context) doctorCheckResource {
	const code, name = "inodes", "Inode headroom"
	if rt.dataDir == "" {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "data directory not configured"}
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(rt.dataDir, &st); err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("could not read inode usage: %s", err)}
	}
	if st.Files == 0 {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "this filesystem does not limit inodes"}
	}
	pct := float64(st.Ffree) * 100 / float64(st.Files)
	evidence := fmt.Sprintf("%d of %d inodes free on %s", st.Ffree, st.Files, rt.dataDir)
	warn := envFloat(envDoctorInodeWarnPercent, defaultDoctorInodeWarnPercent)
	if pct < warn {
		return doctorCheckResource{
			Code: code, Name: name, Status: doctorStatusWarn, Evidence: evidence,
			Message:  fmt.Sprintf("only %.1f%% of inodes free, below the %.0f%% warning threshold", pct, warn),
			Fix:      "find the directory with the most small files (du --inodes -x " + rt.dataDir + " | sort -n | tail), clear it, then re-check",
			DocsPath: "/troubleshooting#disk-space",
		}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: fmt.Sprintf("%.1f%% of inodes free", pct), Evidence: evidence}
}

type dockerRootDirer interface {
	DockerRootDir(ctx context.Context) (string, error)
}

func (rt *Router) doctorCheckDockerRootDisk(ctx context.Context) doctorCheckResource {
	const code, name = "docker_root_disk", "Docker storage disk"
	d, ok := rt.dockerPinger.(dockerRootDirer)
	if !ok {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "the Docker storage root is not available from this client"}
	}
	root, err := d.DockerRootDir(ctx)
	if err != nil || root == "" {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "could not ask Docker for its storage root"}
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(root, &st); err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("cannot read %s from this process (a remote or sandboxed daemon): %s", root, err)}
	}
	free := int64(st.Bavail) * int64(st.Bsize) //nolint:gosec // statfs fields are non-negative
	threshold := rt.doctorDiskWarningBytes
	if threshold <= 0 {
		threshold = defaultDoctorDiskWarningBytes
	}
	evidence := fmt.Sprintf("%s free under %s", diskspace.HumanBytes(free), root)
	if free < threshold {
		return doctorCheckResource{
			Code: code, Name: name, Status: doctorStatusWarn, Evidence: evidence,
			Message:  fmt.Sprintf("%s free, below the %s warning threshold", diskspace.HumanBytes(free), diskspace.HumanBytes(threshold)),
			Fix:      "free space with the Clean up Docker data action, or move Docker's data-root to a larger disk",
			DocsPath: "/troubleshooting#disk-space",
		}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: fmt.Sprintf("%s free", diskspace.HumanBytes(free)), Evidence: evidence}
}

func (rt *Router) doctorCheckOrphans(ctx context.Context) doctorCheckResource {
	const code, name = "docker_orphans", "Orphaned Docker data"
	r, err := rt.fixitReclaimable(ctx)
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("could not inventory Docker data: %s", err)}
	}
	total := r.ContainersBytes + r.ImagesBytes + r.VolumesBytes + r.BuildCacheBytes
	evidence := fmt.Sprintf("containers %s, images %s, anonymous volumes %s, build cache %s, %d orphan network(s)",
		diskspace.HumanBytes(r.ContainersBytes), diskspace.HumanBytes(r.ImagesBytes),
		diskspace.HumanBytes(r.VolumesBytes), diskspace.HumanBytes(r.BuildCacheBytes), len(r.OrphanNetworks))
	limit := envInt64(envDoctorOrphanWarnBytes, defaultDoctorOrphanWarnBytes)
	if total >= limit || len(r.OrphanNetworks) > 0 {
		return doctorCheckResource{
			Code: code, Name: name, Status: doctorStatusWarn, Evidence: evidence,
			Message: fmt.Sprintf("%s of unreferenced data and %d orphan network(s) could be reclaimed", diskspace.HumanBytes(total), len(r.OrphanNetworks)),
			Fix:     "use Clean up Docker data on the Disk space check; rollback targets and volumes in use are kept",
		}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "nothing worth reclaiming", Evidence: evidence}
}

func envInt64(key string, def int64) int64 {
	if n, err := strconv.ParseInt(os.Getenv(key), 10, 64); err == nil && n > 0 {
		return n
	}
	return def
}

func (rt *Router) doctorLookup(ctx context.Context, host string) ([]string, error) {
	if rt.doctorLookupHost != nil {
		return rt.doctorLookupHost(ctx, host)
	}
	return net.DefaultResolver.LookupHost(ctx, host)
}

// doctorCheckDomainDNS is a cheap resolves-or-not summary; per-domain diagnosis lives in the domains doctor.
func (rt *Router) doctorCheckDomainDNS(ctx context.Context) doctorCheckResource {
	const code, name = "dns_domains", "DNS for configured domains"
	owners, err := rt.domainOwners(ctx)
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("could not list domains: %s", err)}
	}
	domains := make([]string, 0, len(owners))
	for d := range owners {
		if !strings.HasPrefix(d, "*.") {
			domains = append(domains, d)
		}
	}
	sort.Strings(domains)
	if len(domains) == 0 {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "no domains configured yet"}
	}
	if len(domains) > doctorDNSMaxDomains {
		domains = domains[:doctorDNSMaxDomains]
	}
	failed := make([]string, len(domains))
	sem := make(chan struct{}, doctorDNSParallelism)
	var wg sync.WaitGroup
	for i, d := range domains {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			lctx, cancel := context.WithTimeout(ctx, doctorDNSLookupTimeout)
			defer cancel()
			if addrs, err := rt.doctorLookup(lctx, d); err != nil || len(addrs) == 0 {
				failed[i] = d
			}
		}()
	}
	wg.Wait()
	var bad []string
	for _, d := range failed {
		if d != "" {
			bad = append(bad, d)
		}
	}
	if len(bad) == 0 {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: fmt.Sprintf("all %d domain(s) resolve", len(domains))}
	}
	return doctorCheckResource{
		Code: code, Name: name, Status: doctorStatusWarn,
		Message:  fmt.Sprintf("%d of %d domain(s) do not resolve", len(bad), len(domains)),
		Evidence: strings.Join(bad, ", "),
		Fix:      rt.cliName() + " domains doctor " + bad[0],
		DocsPath: "/domains-and-ingress",
	}
}

func (rt *Router) doctorCheckCertExpiry(ctx context.Context) doctorCheckResource {
	const code, name = "certificates", "Certificate expiry"
	now := time.Now()
	rows, err := rt.certTruthRows(ctx, now)
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("could not read certificates: %s", err)}
	}
	if len(rows) == 0 {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "no certificates issued yet"}
	}
	shortLived := attention.ShortLivedCertLimit()
	var failing, soon []string
	for _, c := range rows {
		if attention.CertExpiryClass(certDoctorFacts(c), shortLived) == "" {
			continue
		}
		label := fmt.Sprintf("%s (%d days)", c.Info.Domain, c.DaysLeft)
		switch {
		case c.Info.Status == "expired":
			label = c.Info.Domain + " (expired)"
			failing = append(failing, label)
		case c.RenewalFailed() || c.Renewal == "stalled" || c.NearExpiry():
			failing = append(failing, label+certReasonSuffix(c))
		default:
			soon = append(soon, label)
		}
	}
	switch {
	case len(failing) > 0:
		return doctorCheckResource{
			Code: code, Name: name, Status: doctorStatusFail,
			Message:  fmt.Sprintf("%d certificate(s) expired, failing to renew or expiring within %d days", len(failing), certflow.FailDays()),
			Evidence: strings.Join(append(failing, soon...), ", "),
			Fix:      "use Renew now, and check the ACME reachability check above",
			DocsPath: "/troubleshooting#certificates",
		}
	case len(soon) > 0:
		return doctorCheckResource{
			Code: code, Name: name, Status: doctorStatusWarn,
			Message:  fmt.Sprintf("%d certificate(s) expire soon and are not renewing", len(soon)),
			Evidence: strings.Join(soon, ", "),
			Fix:      "open Certificates and renew the listed domains",
			DocsPath: "/troubleshooting#certificates",
		}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: fmt.Sprintf("%d certificate(s), none need attention", len(rows))}
}

func certReasonSuffix(c certTruthRow) string {
	switch {
	case c.RenewalFailed():
		return ", last renewal failed"
	case c.Renewal == "stalled":
		return ", renewal stalled"
	}
	return ""
}

func (rt *Router) doctorCheckBackupFreshness(ctx context.Context) doctorCheckResource {
	const code, name = "backup_freshness", "Database backups on schedule"
	dbs, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("could not list databases: %s", err)}
	}
	grace := envDuration(envAttentionBackupGrace, alerting.DefaultBackupMissingGracePeriod)
	now := time.Now()
	scheduled := 0
	var overdue []string
	for _, d := range dbs {
		if d.BackupSchedule == "" || d.Suspended {
			continue
		}
		scheduled++
		if _, ok := rt.backupOverdueItem(ctx, d, grace, now); ok {
			overdue = append(overdue, d.Name)
		}
	}
	if scheduled == 0 {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "no database has a backup schedule"}
	}
	if len(overdue) > 0 {
		return doctorCheckResource{
			Code: code, Name: name, Status: doctorStatusWarn,
			Message:  fmt.Sprintf("%d of %d scheduled database backup(s) overdue", len(overdue), scheduled),
			Evidence: strings.Join(overdue, ", "),
			Fix:      "open each listed database and run a backup, then check why the schedule missed",
			DocsPath: "/backups",
		}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: fmt.Sprintf("%d scheduled database backup(s) on time", scheduled)}
}

func (rt *Router) agentUnit() string {
	if rt.brand == nil || rt.brand.BinaryName == "" {
		return "agent"
	}
	return rt.brand.BinaryName + "-agent"
}

// doctorCheckNodes reports agent connectivity per remote node plus agent version skew.
func (rt *Router) doctorCheckNodes(ctx context.Context) []doctorCheckResource {
	nodes, err := rt.nodes.ListNodes(ctx)
	if err != nil {
		return []doctorCheckResource{{Code: "nodes", Name: "Nodes", Status: doctorStatusUnknown, Message: fmt.Sprintf("could not list nodes: %s", err)}}
	}
	now := time.Now()
	renewal := rt.renewalStates(ctx)
	var out []doctorCheckResource
	var outdated, mismatched []string
	remote := 0
	for i := range nodes {
		n := nodes[i]
		if rt.isLocalNodeRecord(&n) {
			continue
		}
		remote++
		out = append(out, rt.doctorNodeAgent(n, now))
		if _, agent := rt.nodeCertAndAgent(n, now, renewal); agent != nil {
			switch {
			case agent.Outdated:
				outdated = append(outdated, fmt.Sprintf("%s (%s, minimum %s)", n.Name, orNotSet(agent.Version), agent.MinVersion))
			case agent.Version != "" && agent.Version != agent.ControlPlaneVersion:
				mismatched = append(mismatched, n.Name)
			}
		}
	}
	if remote == 0 {
		return []doctorCheckResource{{Code: "nodes", Name: "Nodes", Status: doctorStatusOK, Message: "single node, no remote agents to check"}}
	}
	skew := doctorCheckResource{Code: "agent_version_skew", Name: "Agent versions", Status: doctorStatusOK, Message: "every agent matches the control plane"}
	switch {
	case len(outdated) > 0:
		skew.Status = doctorStatusWarn
		skew.Message = fmt.Sprintf("%d agent(s) older than the minimum this control plane supports", len(outdated))
		skew.Evidence = strings.Join(outdated, ", ")
		skew.Fix = "upgrade the listed agents from the Nodes page, then re-check"
		skew.DocsPath = "/multi-node"
	case len(mismatched) > 0:
		skew.Message = fmt.Sprintf("%d agent(s) run a different version than the control plane, still supported", len(mismatched))
		skew.Evidence = strings.Join(mismatched, ", ")
	}
	return append(out, skew)
}

func (rt *Router) doctorNodeAgent(n store.Node, now time.Time) doctorCheckResource {
	c := doctorCheckResource{Code: "node_agent_" + n.ID, Name: "Agent on " + n.Name}
	seen := "never"
	if n.LastSeenAt != nil {
		seen = now.Sub(*n.LastSeenAt).Round(time.Second).String() + " ago"
	}
	c.Evidence = fmt.Sprintf("status %s, last seen %s, address %s", n.Status, seen, orNotSet(n.Address))
	fix := "on " + n.Name + ": check the agent service (systemctl status " + rt.agentUnit() + "), and that it can reach this control plane"
	switch {
	case n.Status == store.NodeStatusOnline && (n.LastSeenAt == nil || now.Sub(*n.LastSeenAt) < doctorNodeSilentAfter):
		c.Status, c.Message = doctorStatusOK, "connected"
	case n.Status == store.NodeStatusOnline:
		c.Status, c.Message, c.Fix = doctorStatusWarn, "marked online but silent for "+seen, fix
	case n.Status == store.NodeStatusPending && n.LastSeenAt == nil:
		c.Status, c.Message, c.Fix = doctorStatusWarn, "enrolled but has never connected", fix
	case n.Status == store.NodeStatusPending:
		c.Status, c.Message, c.Fix = doctorStatusWarn, "waiting for the agent to finish enrolling", fix
	default:
		c.Status, c.Message, c.Fix = doctorStatusFail, "agent is not connected", fix
	}
	return c
}

// doctorCheckMesh reports the WireGuard readiness checks, but only on a multi-node install.
func (rt *Router) doctorCheckMesh(ctx context.Context) []doctorCheckResource {
	nodes, err := rt.nodes.ListNodes(ctx)
	if err != nil {
		return nil
	}
	remote := 0
	for i := range nodes {
		if !rt.isLocalNodeRecord(&nodes[i]) {
			remote++
		}
	}
	if remote == 0 {
		return nil
	}
	var out []doctorCheckResource
	for _, c := range network.ReadinessChecks(rt.meshReadinessEnv(rt.mesh != nil)) {
		if c.ID == network.CheckFirewall || c.ID == network.CheckLocalNode {
			continue
		}
		r := doctorCheckResource{Code: c.ID, Name: "Mesh: " + strings.ReplaceAll(c.ID, "_", " "), Message: c.Detail, Fix: c.Hint}
		switch c.Status {
		case network.CheckPass:
			r.Status = doctorStatusOK
		case network.CheckFail:
			r.Status = doctorStatusFail
		default:
			r.Status = doctorStatusWarn
		}
		out = append(out, r)
	}
	return out
}
