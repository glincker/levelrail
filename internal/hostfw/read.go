package hostfw

import (
	"context"
	"regexp"
	"strconv"
	"strings"
)

var (
	ufwRule     = regexp.MustCompile(`^(.*?)\s{2,}(ALLOW|DENY|REJECT|LIMIT)(?:\s+(IN|OUT|FWD))?\s+(.*)$`)
	ufwDefault  = regexp.MustCompile(`(?i)(\w+)\s+\(incoming\)`)
	portRange   = regexp.MustCompile(`^(\d+):(\d+)$`)
	ufwProfiles = map[string][]int{
		"nginx full": {80, 443}, "apache full": {80, 443}, "www full": {80, 443},
		"nginx http": {80}, "apache": {80}, "www": {80},
		"nginx https": {443}, "apache secure": {443}, "www secure": {443},
	}
)

func readUFW(ctx context.Context, run Runner) Status {
	st := newStatus(BackendUFW)
	out, err := run(ctx, "ufw", "status", "verbose")
	if err != nil {
		if unreadable(out, err) {
			return st.cannotRead("ufw status", out, err)
		}
		st.Unknown, st.Detail = true, "ufw status failed: "+firstLine(err.Error())
		return st
	}
	text := string(out)
	if !strings.Contains(text, "Status: active") {
		st.Detail = "ufw is installed but inactive, so it is not filtering traffic"
		return st
	}
	st.Active = true
	policy := "unknown"
	inRules := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, " \r")
		if strings.HasPrefix(line, "Default:") {
			if m := ufwDefault.FindStringSubmatch(line); m != nil {
				policy = strings.ToLower(m[1])
			}
		}
		if strings.HasPrefix(line, "--") {
			inRules = true
			continue
		}
		if !inRules {
			continue
		}
		applyUFWRule(&st, line)
	}
	if policy == "allow" {
		st.Active = false
		st.Open = map[int]bool{80: true, 443: true}
		st.Detail = "ufw is active but its default for incoming traffic is allow, so nothing is blocked"
		return st
	}
	st.Detail = ufwSummary(st)
	return st
}

func applyUFWRule(st *Status, line string) {
	m := ufwRule.FindStringSubmatch(line)
	if m == nil {
		return
	}
	to := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(m[1]), "(v6)"))
	from := strings.TrimSpace(m[4])
	if m[3] == "OUT" || m[3] == "FWD" {
		return
	}
	ports := ufwPorts(to)
	switch m[2] {
	case "ALLOW", "LIMIT":
		if !strings.HasPrefix(from, "Anywhere") {
			return
		}
		for _, p := range WebPorts {
			if containsInt(ports, p) {
				st.Open[p] = true
			}
		}
	default:
		for _, p := range WebPorts {
			if containsInt(ports, p) {
				st.denied = append(st.denied, p)
			}
		}
	}
}

func ufwSummary(st Status) string {
	if st.Blocking() {
		return "ufw is active and does not allow " + joinPorts(st.MissingPorts()) + " from anywhere"
	}
	return "ufw is active and allows ports 80 and 443 from anywhere"
}

func joinPorts(ports []int) string {
	parts := make([]string, len(ports))
	for i, p := range ports {
		parts[i] = strconv.Itoa(p)
	}
	return strings.Join(parts, " and ")
}

// ufwPorts expands a "To" column such as "80,443/tcp", "8000:9000/tcp" or "Nginx Full" into TCP ports.
func ufwPorts(to string) []int {
	if p, ok := ufwProfiles[strings.ToLower(to)]; ok {
		return p
	}
	spec, proto, hasProto := strings.Cut(to, "/")
	if hasProto && proto != "tcp" {
		return nil
	}
	return expandPorts(spec)
}

func expandPorts(spec string) []int {
	var out []int
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if m := portRange.FindStringSubmatch(part); m != nil {
			lo, _ := strconv.Atoi(m[1])
			hi, _ := strconv.Atoi(m[2])
			for _, p := range WebPorts {
				if p >= lo && p <= hi {
					out = append(out, p)
				}
			}
			continue
		}
		if n, err := strconv.Atoi(part); err == nil {
			out = append(out, n)
		}
	}
	return out
}

func containsInt(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func readFirewalld(ctx context.Context, run Runner) Status {
	st := newStatus(BackendFirewalld)
	out, err := run(ctx, "firewall-cmd", "--state")
	state := strings.TrimSpace(string(out))
	switch {
	case err != nil && strings.Contains(state, "not running"):
		st.Detail = "firewalld is installed but not running"
		return st
	case err != nil && unreadable(out, err):
		return st.cannotRead("firewall-cmd --state", out, err)
	case err != nil:
		st.Unknown, st.Detail = true, "firewall-cmd --state failed: "+firstLine(err.Error())
		return st
	case state != "running":
		st.Detail = "firewalld is installed but not running"
		return st
	}
	st.Active = true
	query := func(arg string) bool {
		o, _ := run(ctx, "firewall-cmd", arg)
		return strings.TrimSpace(string(o)) == "yes"
	}
	st.Open[80] = query("--query-service=http") || query("--query-port=80/tcp")
	st.Open[443] = query("--query-service=https") || query("--query-port=443/tcp")
	if st.Blocking() {
		st.Detail = "firewalld is running and does not allow " + joinPorts(st.MissingPorts()) + " in the default zone"
	} else {
		st.Detail = "firewalld is running and allows ports 80 and 443 in the default zone"
	}
	return st
}

var (
	nftTableRe = regexp.MustCompile(`^\s*table\s+(\S+)\s+(\S+)\s*\{`)
	nftChainRe = regexp.MustCompile(`^\s*chain\s+(\S+)\s*\{`)
	nftHookRe  = regexp.MustCompile(`hook\s+input\b.*?policy\s+(\w+)`)
)

type nftTarget struct {
	Family, Table, Chain string
	Ambiguous            bool
}

func readNftables(ctx context.Context, run Runner) Status {
	st := newStatus(BackendNftables)
	out, err := run(ctx, "nft", "list", "ruleset")
	if err != nil {
		if unreadable(out, err) {
			return st.cannotRead("nft list ruleset", out, err)
		}
		st.Unknown, st.Detail = true, "nft list ruleset failed: "+firstLine(err.Error())
		return st
	}
	var family, table, chain string
	accepted := map[string]map[int]bool{}
	var dropChains []nftTarget
	for _, line := range strings.Split(string(out), "\n") {
		if m := nftTableRe.FindStringSubmatch(line); m != nil {
			family, table, chain = m[1], m[2], ""
			continue
		}
		if m := nftChainRe.FindStringSubmatch(line); m != nil {
			chain = m[1]
			continue
		}
		if m := nftHookRe.FindStringSubmatch(line); m != nil && strings.EqualFold(m[1], "drop") {
			dropChains = append(dropChains, nftTarget{Family: family, Table: table, Chain: chain})
			continue
		}
		if strings.Contains(line, "accept") && strings.Contains(line, "dport") && !strings.Contains(line, "saddr") {
			key := family + "/" + table
			if accepted[key] == nil {
				accepted[key] = map[int]bool{}
			}
			for _, p := range nftAcceptedPorts(line) {
				accepted[key][p] = true
			}
		}
	}
	if len(dropChains) == 0 {
		st.Detail = "nftables has no input chain that drops traffic, so it is not blocking web ports"
		return st
	}
	st.Active = true
	st.Open = map[int]bool{80: true, 443: true}
	var missingTarget []nftTarget
	for _, c := range dropChains {
		for _, p := range WebPorts {
			if !accepted[c.Family+"/"+c.Table][p] {
				st.Open[p] = false
				missingTarget = append(missingTarget, c)
			}
		}
	}
	if len(missingTarget) > 0 {
		t := missingTarget[0]
		for _, o := range missingTarget {
			if o != t {
				t.Ambiguous = true
			}
		}
		st.nft = t
	}
	if st.Blocking() {
		st.Detail = "nftables drops incoming traffic and has no accept rule for " + joinPorts(st.MissingPorts())
	} else {
		st.Detail = "nftables accepts incoming ports 80 and 443"
	}
	return st
}

func nftAcceptedPorts(line string) []int {
	var out []int
	i := strings.Index(line, "dport")
	rest := strings.TrimSpace(line[i+len("dport"):])
	if strings.HasPrefix(rest, "{") {
		end := strings.Index(rest, "}")
		if end < 0 {
			return nil
		}
		for _, p := range strings.Split(rest[1:end], ",") {
			out = append(out, nftPortOrRange(strings.TrimSpace(p))...)
		}
		return out
	}
	f := strings.Fields(rest)
	if len(f) > 0 {
		out = append(out, nftPortOrRange(f[0])...)
	}
	return out
}

func nftPortOrRange(s string) []int {
	if lo, hi, ok := strings.Cut(s, "-"); ok {
		a, e1 := strconv.Atoi(lo)
		b, e2 := strconv.Atoi(hi)
		if e1 != nil || e2 != nil {
			return nil
		}
		var out []int
		for _, p := range WebPorts {
			if p >= a && p <= b {
				out = append(out, p)
			}
		}
		return out
	}
	if n, err := strconv.Atoi(s); err == nil {
		return []int{n}
	}
	return nil
}

var (
	iptPolicy   = regexp.MustCompile(`^-P INPUT (\w+)`)
	iptCatchAll = regexp.MustCompile(`^-A INPUT -j (REJECT|DROP)\b`)
	iptDport    = regexp.MustCompile(`--dports?\s+(\S+)`)
)

func readIptables(ctx context.Context, run Runner) Status {
	st := newStatus(BackendIptables)
	out, err := run(ctx, "iptables", "-S", "INPUT")
	if err != nil {
		if unreadable(out, err) {
			return st.cannotRead("iptables -S", out, err)
		}
		st.Unknown, st.Detail = true, "iptables -S failed: "+firstLine(err.Error())
		return st
	}
	policyDrop, catchAllAt, rule := false, 0, 0
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if m := iptPolicy.FindStringSubmatch(line); m != nil {
			policyDrop = m[1] == "DROP" || m[1] == "REJECT"
			continue
		}
		if !strings.HasPrefix(line, "-A INPUT") {
			continue
		}
		rule++
		if iptCatchAll.MatchString(line) && catchAllAt == 0 {
			catchAllAt = rule
		}
		if strings.Contains(line, "-j ACCEPT") && !strings.Contains(line, " -s ") && !strings.Contains(line, "-p udp") {
			if m := iptDport.FindStringSubmatch(line); m != nil {
				for _, p := range WebPorts {
					if containsInt(expandPorts(m[1]), p) {
						st.Open[p] = true
					}
				}
			}
		}
	}
	if !policyDrop && catchAllAt == 0 {
		st.Open = map[int]bool{80: true, 443: true}
		st.Detail = "iptables accepts incoming traffic by default, so it is not blocking web ports"
		return st
	}
	st.Active = true
	st.insertAt = 1
	if st.Blocking() {
		st.Detail = "iptables drops incoming traffic and has no accept rule for " + joinPorts(st.MissingPorts())
	} else {
		st.Detail = "iptables accepts incoming ports 80 and 443"
	}
	return st
}
