// Package hostfw reads the host firewall (ufw, firewalld, nftables, iptables)
// and opens exactly ports 80 and 443 in it, nothing else. It never enables a
// disabled firewall, never edits or deletes a rule it did not add, and every
// change it makes has a printed undo.
package hostfw

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Backend names a firewall tool.
type Backend string

// Backend values, in detection order.
const (
	BackendNone      Backend = "none"
	BackendUFW       Backend = "ufw"
	BackendFirewalld Backend = "firewalld"
	BackendNftables  Backend = "nftables"
	BackendIptables  Backend = "iptables"
)

// WebPorts are the only ports this package ever opens.
var WebPorts = []int{80, 443}

// ErrRefused is wrapped by Apply when the change is not safe to make here.
var ErrRefused = errors.New("hostfw: refused")

// Runner runs one command and returns its combined output.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Deps are the host seams; tests replace both.
type Deps struct {
	LookPath func(string) (string, error)
	Run      Runner
}

// SystemDeps runs the real tools.
func SystemDeps() Deps {
	return Deps{LookPath: exec.LookPath, Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, name, args...).CombinedOutput() //nolint:gosec // name and args come from this package's fixed command templates
	}}
}

// Status is what the host firewall does with ports 80 and 443 right now.
type Status struct {
	Backend Backend `json:"backend"`
	// Installed is true when a firewall tool exists on the host.
	Installed bool `json:"installed"`
	// Active is true when the firewall filters incoming traffic.
	Active bool `json:"active"`
	// Unknown is true when a tool exists but could not be read, usually for lack of privileges.
	Unknown bool `json:"unknown"`
	// Open reports, per web port, whether incoming TCP is allowed.
	Open   map[int]bool `json:"open"`
	Detail string       `json:"detail"`

	nft nftTarget
	// denied lists web ports with an explicit deny rule.
	denied []int
	// insertAt is where an iptables accept rule must go to precede a catch-all reject.
	insertAt int
}

// Blocking is true when an active firewall would drop traffic to a web port.
func (s Status) Blocking() bool {
	if !s.Active || s.Unknown {
		return false
	}
	for _, p := range WebPorts {
		if !s.Open[p] {
			return true
		}
	}
	return false
}

// MissingPorts lists the web ports an active firewall does not allow.
func (s Status) MissingPorts() []int {
	var out []int
	if !s.Active || s.Unknown {
		return out
	}
	for _, p := range WebPorts {
		if !s.Open[p] {
			out = append(out, p)
		}
	}
	return out
}

func newStatus(b Backend) Status {
	return Status{Backend: b, Installed: true, Open: map[int]bool{80: false, 443: false}}
}

func unreadable(out []byte, err error) bool {
	if err == nil {
		return false
	}
	l := strings.ToLower(string(out) + err.Error())
	return strings.Contains(l, "permission") || strings.Contains(l, "denied") || strings.Contains(l, "root") ||
		strings.Contains(l, "authorization") || strings.Contains(l, "not permitted") || strings.Contains(l, "executable file not found")
}

func (s Status) cannotRead(name string, out []byte, err error) Status {
	s.Unknown = true
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		msg = err.Error()
	}
	s.Detail = fmt.Sprintf("%s could not be read from this process (%s); it usually needs root", name, firstLine(msg))
	return s
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// Detect reads every installed firewall tool and reports the one that is
// enforcing, else the first that is installed, else BackendNone.
func Detect(ctx context.Context, d Deps) Status {
	probes := []struct {
		tool string
		read func(context.Context, Runner) Status
	}{
		{"ufw", readUFW}, {"firewall-cmd", readFirewalld}, {"nft", readNftables}, {"iptables", readIptables},
	}
	var first *Status
	var unknown *Status
	for _, p := range probes {
		if _, err := d.LookPath(p.tool); err != nil {
			continue
		}
		st := p.read(ctx, d.Run)
		if st.Active || st.Blocking() {
			return st
		}
		if st.Unknown && unknown == nil {
			cp := st
			unknown = &cp
		}
		if first == nil {
			cp := st
			first = &cp
		}
	}
	switch {
	case unknown != nil:
		return *unknown
	case first != nil:
		return *first
	}
	return Status{Backend: BackendNone, Open: map[int]bool{}, Detail: "no firewall tool (ufw, firewalld, nftables, iptables) is visible from this process"}
}
