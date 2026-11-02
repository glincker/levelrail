package hostfw

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// OpenCommand is text for the operator to run; this package never executes it.
type OpenCommand struct {
	Port    int     `json:"port"`
	Backend Backend `json:"backend"`
	Command string  `json:"command"`
	Note    string  `json:"note,omitempty"`
}

// ValidPort reports whether p is a TCP port number.
func ValidPort(p int) bool { return p >= 1 && p <= 65535 }

var openBackends = []Backend{BackendUFW, BackendFirewalld, BackendNftables, BackendIptables}

// serviceUnit maps a backend to the systemd unit that says it is enforcing.
var serviceUnit = map[Backend]string{BackendUFW: "ufw", BackendFirewalld: "firewalld", BackendNftables: "nftables"}

const probeTimeout = 3 * time.Second

// DetectOpenBackend picks the backend whose command to print, read only.
// firewalld beats nftables, ufw beats iptables; a bare iptables binary counts
// last, and BackendNone means the caller should list every option.
func DetectOpenBackend(ctx context.Context, d Deps) Backend {
	tools := map[Backend]string{BackendUFW: "ufw", BackendFirewalld: "firewall-cmd", BackendNftables: "nft", BackendIptables: "iptables"}
	active := func(b Backend) bool {
		if _, err := d.LookPath(tools[b]); err != nil {
			return false
		}
		if b == BackendIptables {
			return true
		}
		cctx, cancel := context.WithTimeout(ctx, probeTimeout)
		defer cancel()
		out, err := d.Run(cctx, "systemctl", "is-active", serviceUnit[b])
		return err == nil && strings.TrimSpace(string(out)) == "active"
	}
	for _, b := range []Backend{BackendFirewalld, BackendUFW, BackendNftables, BackendIptables} {
		if active(b) {
			return b
		}
	}
	return BackendNone
}

// OpenCommands returns one command per valid port for backend b, or one per
// backend and port when b is BackendNone. Invalid ports are dropped, and the
// command string is built from integers only.
func OpenCommands(b Backend, ports []int) []OpenCommand {
	var out []OpenCommand
	seen := map[int]bool{}
	for _, p := range ports {
		if !ValidPort(p) || seen[p] {
			continue
		}
		seen[p] = true
		if b == BackendNone {
			for _, ob := range openBackends {
				c := openCommand(ob, p)
				c.Note = "no active firewall detected, option for " + string(ob) + ". " + c.Note
				out = append(out, c)
			}
			continue
		}
		if c := openCommand(b, p); c.Command != "" {
			out = append(out, c)
		}
	}
	return out
}

func openCommand(b Backend, p int) OpenCommand {
	n := strconv.Itoa(p)
	c := OpenCommand{Port: p, Backend: b}
	switch b {
	case BackendUFW:
		c.Command = "ufw allow " + n + "/tcp"
		c.Note = "ufw keeps the rule across reboots."
	case BackendFirewalld:
		c.Command = "firewall-cmd --permanent --add-port=" + n + "/tcp && firewall-cmd --reload"
		c.Note = "--permanent stores the rule, the reload applies it."
	case BackendNftables:
		c.Command = "nft add rule inet filter input tcp dport " + n + " accept"
		c.Note = "Table and chain names depend on your ruleset; check nft list ruleset. Persist it in /etc/nftables.conf (or your distro's equivalent)."
	case BackendIptables:
		c.Command = "iptables -I INPUT -p tcp --dport " + n + " -j ACCEPT"
		c.Note = "Not persistent: save with netfilter-persistent save (Debian) or service iptables save (RHEL)."
	}
	return c
}
