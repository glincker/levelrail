package preflight

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"github.com/GLINCKER/levelrail/internal/bindmount"
	"github.com/GLINCKER/levelrail/internal/diskspace"
)

func pass(id, name, reason string) Check {
	return Check{ID: id, Name: name, Status: StatusPass, Reason: reason}
}

func fail(id, name, reason, fix string) Check {
	return Check{ID: id, Name: name, Status: StatusFail, Reason: reason, Fix: fix}
}

func warn(id, name, reason, fix string) Check {
	return Check{ID: id, Name: name, Status: StatusWarn, Reason: reason, Fix: fix}
}

var cloudflareRanges = mustPrefixes(
	"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22",
	"141.101.64.0/18", "108.162.192.0/18", "190.93.240.0/20", "188.114.96.0/20",
	"197.234.240.0/22", "198.41.128.0/17", "162.158.0.0/15", "104.16.0.0/13",
	"104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
	"2400:cb00::/32", "2606:4700::/32", "2803:f800::/32", "2405:b500::/32",
	"2405:8100::/32", "2a06:98c0::/29", "2c0f:f248::/32",
)

func mustPrefixes(ss ...string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(ss))
	for _, s := range ss {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}

func isCloudflare(addr netip.Addr) bool {
	for _, p := range cloudflareRanges {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

func checkDomains(ctx context.Context, req Request, env Env) []Check {
	if len(req.Domains) == 0 || env.LookupHost == nil {
		return nil
	}
	var serverIP string
	if env.PublicIP != nil {
		if ip, err := env.PublicIP(ctx); err == nil {
			serverIP = ip
		}
	}
	var out []Check
	for _, d := range req.Domains {
		out = append(out, checkDomain(ctx, d, serverIP, env))
	}
	return out
}

func checkDomain(ctx context.Context, domain, serverIP string, env Env) Check {
	id, name := "dns:"+domain, "DNS for "+domain
	host := strings.TrimPrefix(domain, "*.")
	addrs, err := env.LookupHost(ctx, host)
	if err != nil || len(addrs) == 0 {
		return warn(id, name, "the domain does not resolve yet",
			"Create an A record for "+domain+pointTo(serverIP)+", then wait for DNS to propagate. TLS issuance fails until it resolves.")
	}
	var parsed []netip.Addr
	for _, a := range addrs {
		if p, perr := netip.ParseAddr(a); perr == nil {
			parsed = append(parsed, p.Unmap())
		}
	}
	proxied := false
	for _, a := range parsed {
		if isCloudflare(a) {
			proxied = true
		}
	}
	if proxied {
		return warn(id, name, "the domain resolves to Cloudflare, so it is proxied (orange cloud) and the real server IP is hidden",
			"This works if Cloudflare SSL mode is Full (strict) and the origin certificate is trusted. If certificate issuance fails, switch the record to DNS only (grey cloud) until the certificate is issued.")
	}
	if serverIP == "" {
		return warn(id, name, "the domain resolves to "+strings.Join(addrs, ", ")+" but this server's public IP could not be detected to compare",
			"Confirm the record points at this server.")
	}
	want, perr := netip.ParseAddr(serverIP)
	for _, a := range parsed {
		if perr == nil && a == want.Unmap() {
			return pass(id, name, "resolves to this server ("+serverIP+")")
		}
	}
	return fail(id, name, "the domain resolves to "+strings.Join(addrs, ", ")+" but this server's public IP is "+serverIP,
		"Point the A record for "+domain+" at "+serverIP+" and remove stale records, otherwise certificate issuance fails.")
}

func pointTo(ip string) string {
	if ip == "" {
		return " pointing at this server"
	}
	return " pointing at " + ip
}

func checkHostPort(ctx context.Context, req Request, env Env) []Check {
	if req.HostPort <= 0 || env.HostPortHolder == nil {
		return nil
	}
	id, name := "host_port", fmt.Sprintf("Host port %d", req.HostPort)
	holder, inUse, err := env.HostPortHolder(ctx, req.NodeID, req.HostPort)
	if err != nil {
		return []Check{warn(id, name, "could not verify whether the port is free: "+err.Error(), "")}
	}
	if inUse {
		who := "another process"
		if holder != "" {
			who = holder
		}
		return []Check{fail(id, name, fmt.Sprintf("port %d on the target node is already in use by %s", req.HostPort, who),
			"Pick a different host port, or leave it unpinned so one is assigned automatically.")}
	}
	return []Check{pass(id, name, "free on the target node")}
}

func checkImageAndDisk(ctx context.Context, req Request, env Env) []Check {
	if req.Image == "" {
		return nil
	}
	var out []Check
	var size int64
	if env.Image != nil {
		id, name := "image", "Image "+req.Image
		info, err := env.Image.Inspect(ctx, req.Image)
		switch {
		case err == nil:
			size = info.SizeBytes
			out = append(out, pass(id, name, "the registry has this image"+sizeNote(size)))
		case errors.Is(err, ErrImageNotFound):
			out = append(out, fail(id, name, "the registry reports no such image or tag",
				"Check the name and tag for typos, and that the tag was pushed."))
		case errors.Is(err, ErrImageUnauthorized):
			out = append(out, warn(id, name, "the registry needs credentials to read this image (or the name is wrong)",
				"Add a registry credential for this registry before deploying a private image."))
		case errors.Is(err, ErrImageRateLimited):
			out = append(out, warn(id, name, "the registry is rate limiting anonymous pulls",
				"Add registry credentials so pulls are authenticated, or retry later."))
		default:
			out = append(out, warn(id, name, "could not reach the registry: "+err.Error(), "Check the node's outbound network access."))
		}
	}
	if env.FreeDisk != nil {
		out = append(out, diskCheck(ctx, req, env, size))
	}
	return out
}

func sizeNote(size int64) string {
	if size <= 0 {
		return ""
	}
	return " (" + diskspace.HumanBytes(size) + " compressed)"
}

func diskCheck(ctx context.Context, req Request, env Env, imageSize int64) Check {
	id, name := "disk", "Disk space"
	free, err := env.FreeDisk(ctx, req.NodeID)
	if err != nil {
		return warn(id, name, "could not read free disk space: "+err.Error(), "")
	}
	if imageSize > 0 {
		need := int64(float64(imageSize) * env.Limits.DiskFactor)
		if free < need {
			return fail(id, name, fmt.Sprintf("%s free but the image needs about %s once pulled and extracted", diskspace.HumanBytes(free), diskspace.HumanBytes(need)),
				"Free space on the node (prune unused images and build cache) or add disk.")
		}
	}
	if free < env.Limits.MinFreeDisk {
		return warn(id, name, fmt.Sprintf("only %s free on the node", diskspace.HumanBytes(free)),
			"Prune unused images and build cache, or add disk, before it runs out mid deploy.")
	}
	return pass(id, name, diskspace.HumanBytes(free)+" free")
}

func checkMemory(ctx context.Context, req Request, env Env) []Check {
	if req.MemoryBytes <= 0 {
		return nil
	}
	id, name := "memory", "Memory limit"
	if req.MemoryBytes < env.Limits.MinMemoryBytes {
		return []Check{warn(id, name, fmt.Sprintf("the %s limit is below the recommended minimum of %s",
			diskspace.HumanBytes(req.MemoryBytes), diskspace.HumanBytes(env.Limits.MinMemoryBytes)),
			"Most runtimes need more than this to start. Raise the memory limit.")}
	}
	if env.Memory == nil {
		return []Check{pass(id, name, diskspace.HumanBytes(req.MemoryBytes)+" limit")}
	}
	total, avail, err := env.Memory(ctx, req.NodeID)
	if err != nil {
		return []Check{pass(id, name, diskspace.HumanBytes(req.MemoryBytes)+" limit (node memory unknown)")}
	}
	if total > 0 && req.MemoryBytes > total {
		return []Check{fail(id, name, fmt.Sprintf("the %s limit is more than the node's total %s", diskspace.HumanBytes(req.MemoryBytes), diskspace.HumanBytes(total)),
			"Lower the memory limit or place the app on a larger node.")}
	}
	if avail > 0 && req.MemoryBytes > avail {
		return []Check{warn(id, name, fmt.Sprintf("the %s limit is more than the %s currently free on the node", diskspace.HumanBytes(req.MemoryBytes), diskspace.HumanBytes(avail)),
			"Other apps are using the memory. Expect OOM kills unless you free memory or use a larger node.")}
	}
	return []Check{pass(id, name, diskspace.HumanBytes(req.MemoryBytes)+" limit fits on the node")}
}

func checkGit(ctx context.Context, req Request, env Env) []Check {
	if req.GitURL == "" || env.Git == nil {
		return nil
	}
	id, name := "git", "Git source"
	err := env.Git.Check(ctx, req.GitURL, req.GitBranch)
	switch {
	case err == nil:
		return []Check{pass(id, name, "repository is reachable"+branchNote(req.GitBranch))}
	case errors.Is(err, ErrGitBranchMissing):
		return []Check{fail(id, name, fmt.Sprintf("the repository has no branch %q", req.GitBranch), "Pick an existing branch.")}
	case errors.Is(err, ErrGitAuth):
		return []Check{warn(id, name, "the repository is not readable anonymously (private, or the URL is wrong)",
			"Connect a git provider or add a deploy key so deploys can clone it.")}
	case errors.Is(err, ErrGitUnverifiable):
		return []Check{warn(id, name, "this URL scheme cannot be checked from here", "")}
	default:
		return []Check{fail(id, name, "could not reach the repository: "+err.Error(), "Check the URL and the host's network access.")}
	}
}

func branchNote(branch string) string {
	if branch == "" {
		return ""
	}
	return " and branch " + branch + " exists"
}

func checkEnv(req Request) []Check {
	if len(req.RequiredEnv) == 0 {
		return nil
	}
	have := make(map[string]bool, len(req.EnvKeys))
	for _, k := range req.EnvKeys {
		have[k] = true
	}
	var missing []string
	for _, k := range req.RequiredEnv {
		if !have[k] {
			missing = append(missing, k)
		}
	}
	id, name := "env", "Required environment variables"
	if len(missing) > 0 {
		return []Check{fail(id, name, "not set: "+strings.Join(missing, ", "), "Set these variables (as env or secrets) before deploying.")}
	}
	return []Check{pass(id, name, "all required variables are set")}
}

func checkMounts(req Request) []Check {
	if len(req.BindMounts) == 0 && len(req.VolumePaths) == 0 {
		return nil
	}
	id, name := "mounts", "Volumes and bind mounts"
	for _, p := range req.BindMounts {
		if err := bindmount.ValidateHostPath(p); err != nil {
			return []Check{fail(id, name, err.Error(), "Use an absolute host path outside protected system directories.")}
		}
	}
	for _, p := range req.VolumePaths {
		if !strings.HasPrefix(p, "/") {
			return []Check{fail(id, name, fmt.Sprintf("volume mount path %q must be absolute", p), "Use an absolute path inside the container.")}
		}
	}
	return []Check{pass(id, name, "mount paths are valid")}
}

func checkGPU(ctx context.Context, req Request, env Env) []Check {
	if !req.GPU || env.GPUFit == nil {
		return nil
	}
	id, name := "gpu", "GPU"
	if err := env.GPUFit(ctx, req.NodeID); err != nil {
		return []Check{fail(id, name, err.Error(), "Place the app on a node with a free GPU, or remove the GPU request.")}
	}
	return []Check{pass(id, name, "a GPU is available on the target node")}
}
