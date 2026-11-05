package hostfw

import (
	"strconv"
	"strings"
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

// OpenFix joins the open commands for b into one sudo line, so the doctor and
// the host card print the same text. firewalld gets a single reload at the end.
func OpenFix(b Backend, ports []int) string {
	cmds := OpenCommands(b, ports)
	if len(cmds) == 0 {
		return ""
	}
	if b == BackendFirewalld {
		var adds []string
		for _, c := range cmds {
			adds = append(adds, "--add-port="+strconv.Itoa(c.Port)+"/tcp")
		}
		return "sudo firewall-cmd --permanent " + strings.Join(adds, " ") + " && sudo firewall-cmd --reload"
	}
	parts := make([]string, 0, len(cmds))
	for _, c := range cmds {
		parts = append(parts, "sudo "+c.Command)
	}
	return strings.Join(parts, " && ")
}
