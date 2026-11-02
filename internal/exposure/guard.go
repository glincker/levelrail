package exposure

import (
	"context"
	"fmt"
	"net/netip"
	"os/exec"
	"slices"
	"strconv"
	"strings"
)

// DetectSSHPorts returns the ports sshd listens on, so a restriction can
// never cut the operator's way in even when sshd is not on port 22.
func (m *Manager) DetectSSHPorts(ctx context.Context) []int {
	if m.sshProbe == nil {
		return nil
	}
	ports := m.sshProbe(ctx)
	slices.Sort(ports)
	return slices.Compact(ports)
}

// Guard lists the reasons a normalized restriction could lock the operator out:
// a detected sshd port, or an allow-list that excludes the caller's own address.
// It returns nothing when the restriction is safe.
func (m *Manager) Guard(ctx context.Context, r Restriction, caller string) []string {
	var out []string
	if slices.Contains(m.DetectSSHPorts(ctx), r.Port) {
		out = append(out, fmt.Sprintf("port %d is where sshd listens, restricting it can cut off your SSH access", r.Port))
	}
	if ip, err := netip.ParseAddr(strings.TrimSpace(caller)); err == nil {
		ip = ip.Unmap()
		if ip.Is4() && !ip.IsLoopback() && !coversAddr(r.Allow, ip) {
			out = append(out, fmt.Sprintf("your own address %s is not in the allow-list, so you would lose access to port %d", ip, r.Port))
		}
	}
	return out
}

func coversAddr(allow []string, ip netip.Addr) bool {
	for _, a := range allow {
		if p, err := netip.ParsePrefix(a); err == nil && p.Contains(ip) {
			return true
		}
	}
	return false
}

// probeSSHPorts asks sshd for its effective config and falls back to the
// listening sockets, because sshd -T needs root and may not be installed.
func (m *Manager) probeSSHPorts(ctx context.Context) []int {
	if m.goos != "linux" {
		return nil
	}
	if out, err := m.run(ctx, "sshd", "-T"); err == nil {
		if ports := parseSSHDConfig(string(out)); len(ports) > 0 {
			return ports
		}
	}
	out, err := exec.CommandContext(ctx, "ss", "-H", "-ltnp").Output() //nolint:gosec // fixed binary and flags
	if err != nil {
		return nil
	}
	return parseSSListeners(string(out))
}

// parseSSHDConfig reads the "port N" lines of sshd -T output.
func parseSSHDConfig(out string) []int {
	var ports []int
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.EqualFold(f[0], "port") {
			if n, err := strconv.Atoi(f[1]); err == nil && n > 0 && n < 65536 {
				ports = append(ports, n)
			}
		}
	}
	return ports
}

// parseSSListeners picks the local ports of sockets owned by sshd from ss -ltnp.
func parseSSListeners(out string) []int {
	var ports []int
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, `"sshd"`) {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		local := f[3]
		i := strings.LastIndexByte(local, ':')
		if i < 0 {
			continue
		}
		if n, err := strconv.Atoi(local[i+1:]); err == nil && n > 0 && n < 65536 {
			ports = append(ports, n)
		}
	}
	return ports
}

// SetSSHProbe replaces the sshd port detection (tests).
func (m *Manager) SetSSHProbe(f func(context.Context) []int) { m.sshProbe = f }
