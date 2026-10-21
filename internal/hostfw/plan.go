package hostfw

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Command is one program invocation, shown to the operator before it runs.
type Command struct {
	Name string   `json:"name"`
	Args []string `json:"args"`
}

// String is the command as typed in a shell.
func (c Command) String() string { return strings.TrimSpace(c.Name + " " + strings.Join(c.Args, " ")) }

// Plan is exactly what opening the web ports would run and how to reverse it.
type Plan struct {
	Backend Backend   `json:"backend"`
	Ports   []int     `json:"ports"`
	Add     []Command `json:"add"`
	Undo    []Command `json:"undo"`
	// Persistent is false when the rule is lost on reboot unless the operator saves the ruleset.
	Persistent bool   `json:"persistent"`
	Note       string `json:"note,omitempty"`
	// Refusal is set when the change must not be made from here; Add is then empty.
	Refusal string `json:"refusal,omitempty"`
}

func tcp(p int) string { return strconv.Itoa(p) + "/tcp" }

// PlanOpen builds the plan for st. It fails closed: a firewall that is off,
// unreadable, ambiguous or holding an explicit deny yields a Refusal or an
// empty plan, never a guess.
func PlanOpen(st Status) Plan {
	pl := Plan{Backend: st.Backend, Persistent: true}
	missing := st.MissingPorts()
	pl.Ports = missing
	switch {
	case st.Unknown:
		pl.Refusal = "the firewall could not be read from this process, so nothing was changed"
		return pl
	case !st.Installed || st.Backend == BackendNone:
		pl.Refusal = "no firewall tool is visible from this process, so there is nothing to open here"
		return pl
	case !st.Active:
		pl.Note = "the firewall is not enabled, so it is not blocking anything; it was left disabled"
		return pl
	case len(missing) == 0:
		return pl
	case len(st.denied) > 0:
		pl.Refusal = "an explicit deny rule for port " + joinPorts(st.denied) + " exists; remove it yourself, it was not touched"
		return pl
	}
	switch st.Backend {
	case BackendUFW:
		for _, p := range missing {
			pl.Add = append(pl.Add, Command{"ufw", []string{"allow", tcp(p)}})
			pl.Undo = append(pl.Undo, Command{"ufw", []string{"delete", "allow", tcp(p)}})
		}
	case BackendFirewalld:
		for _, p := range missing {
			svc := map[int]string{80: "http", 443: "https"}[p]
			pl.Add = append(pl.Add, Command{"firewall-cmd", []string{"--permanent", "--add-service=" + svc}})
			pl.Undo = append(pl.Undo, Command{"firewall-cmd", []string{"--permanent", "--remove-service=" + svc}})
		}
		reload := Command{"firewall-cmd", []string{"--reload"}}
		pl.Add, pl.Undo = append(pl.Add, reload), append(pl.Undo, reload)
	case BackendIptables:
		for _, p := range missing {
			rule := []string{"INPUT", "-p", "tcp", "--dport", strconv.Itoa(p), "-j", "ACCEPT"}
			pl.Add = append(pl.Add, Command{"iptables", append([]string{"-I", "INPUT", strconv.Itoa(max(st.insertAt, 1))}, rule[1:]...)})
			pl.Undo = append(pl.Undo, Command{"iptables", append([]string{"-D"}, rule...)})
		}
		pl.Persistent = false
		pl.Note = "the rule applies now and is lost on reboot unless you save your iptables rules"
	case BackendNftables:
		t := st.nft
		if t.Ambiguous || t.Table == "" {
			pl.Refusal = "more than one nftables input chain drops traffic, so it is unclear where the rule belongs; nothing was changed"
			return pl
		}
		pl.Persistent = false
		pl.Note = "the rule applies now and is lost on reboot unless you save your nftables ruleset"
		for _, p := range missing {
			pl.Add = append(pl.Add, Command{"nft", []string{"-e", "-a", "insert", "rule", t.Family, t.Table, t.Chain, "tcp", "dport", strconv.Itoa(p), "accept"}})
		}
	}
	return pl
}

// Result is what Apply did.
type Result struct {
	Plan   Plan   `json:"plan"`
	Status Status `json:"status"`
	// Undo are the commands that reverse exactly what was added.
	Undo []Command `json:"undo"`
}

var nftHandle = regexp.MustCompile(`# handle (\d+)`)

// Apply re-reads the firewall, then runs the plan. On any failure it undoes
// what it already added and returns the error, so a half-applied run does not
// stay behind. It never enables a disabled firewall.
func Apply(ctx context.Context, d Deps) (Result, error) {
	st := Detect(ctx, d)
	pl := PlanOpen(st)
	res := Result{Plan: pl, Status: st}
	if pl.Refusal != "" {
		return res, fmt.Errorf("%w: %s", ErrRefused, pl.Refusal)
	}
	var done []Command
	rollback := func() {
		for i := len(done) - 1; i >= 0; i-- {
			_, _ = d.Run(ctx, done[i].Name, done[i].Args...)
		}
	}
	for i, c := range pl.Add {
		out, err := d.Run(ctx, c.Name, c.Args...)
		if err != nil {
			rollback()
			return res, fmt.Errorf("hostfw: run %q: %w (%s); undone", c.String(), err, firstLine(strings.TrimSpace(string(out))))
		}
		if pl.Backend == BackendNftables {
			if m := nftHandle.FindStringSubmatch(string(out)); m != nil {
				t := st.nft
				u := Command{"nft", []string{"delete", "rule", t.Family, t.Table, t.Chain, "handle", m[1]}}
				done = append(done, u)
				res.Undo = append(res.Undo, u)
			}
			continue
		}
		if i < len(pl.Undo) {
			done = append(done, pl.Undo[i])
		}
	}
	if pl.Backend != BackendNftables {
		res.Undo = pl.Undo
	}
	res.Status = Detect(ctx, d)
	if res.Status.Blocking() {
		rollback()
		return res, fmt.Errorf("hostfw: ports still blocked after the change, so it was undone: %s", res.Status.Detail)
	}
	return res, nil
}
