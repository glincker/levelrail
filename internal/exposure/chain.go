package exposure

import (
	"strconv"
	"strings"
)

// Chain is the DOCKER-USER chain as read from the host.
type Chain struct {
	// Readable is false when the chain could not be read; Reason says why.
	Readable bool
	Reason   string
	Rules    []ChainRule
}

// ChainRule is one parsed DOCKER-USER rule. Anything the parser does not
// fully understand sets Complex, so classification never trusts it.
type ChainRule struct {
	Raw           string
	Proto         string
	DPort         int
	OrigDPort     int
	Source        string
	SourceNegated bool
	Target        string
	Comment       string
	Complex       bool
}

// Rule targets the classifier acts on.
const (
	targetDrop   = "DROP"
	targetReject = "REJECT"
	targetReturn = "RETURN"
	targetAccept = "ACCEPT"
)

func (r ChainRule) terminal() bool { return r.Target == targetDrop || r.Target == targetReject }
func (r ChainRule) passes() bool   { return r.Target == targetReturn || r.Target == targetAccept }

// ParseChain parses `iptables -S DOCKER-USER` output.
func ParseChain(out string) Chain {
	c := Chain{Readable: true}
	for _, line := range strings.Split(out, "\n") {
		spec, ok := strings.CutPrefix(strings.TrimSpace(line), "-A DOCKER-USER ")
		if !ok {
			continue
		}
		c.Rules = append(c.Rules, parseRule(spec))
	}
	return c
}

func parseRule(spec string) ChainRule {
	r := ChainRule{Raw: spec}
	toks := splitQuoted(spec)
	negate := false
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		arg := func() string {
			if i+1 < len(toks) {
				i++
				return toks[i]
			}
			r.Complex = true
			return ""
		}
		switch t {
		case "!":
			negate = true
			continue
		case "-p", "--protocol":
			r.Proto = arg()
		case "-s", "--source":
			r.Source, r.SourceNegated = arg(), negate
		case "--dport", "--destination-port":
			r.DPort, _ = strconv.Atoi(arg())
		case "--ctorigdstport":
			r.OrigDPort, _ = strconv.Atoi(arg())
		case "--comment":
			r.Comment = arg()
		case "-j", "--jump":
			r.Target = arg()
		case "-m", "--match":
			if m := arg(); m != "tcp" && m != "udp" && m != "conntrack" && m != "comment" {
				r.Complex = true
			}
		case "--ctdir", "--ctstate":
			_ = arg()
		default:
			r.Complex = true
		}
		negate = false
	}
	switch r.Target {
	case targetDrop, targetReject, targetReturn, targetAccept:
	default:
		r.Complex = true
	}
	return r
}

func splitQuoted(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for _, ch := range s {
		switch {
		case ch == '"':
			inQuote = !inQuote
		case ch == ' ' && !inQuote:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(ch)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// applies reports whether r is about the published (hostPort, containerPort,
// proto) binding. DOCKER-USER sees traffic after DNAT, so a plain --dport
// carries the container port while --ctorigdstport carries the host port.
func (r ChainRule) applies(hostPort, containerPort int, proto string) bool {
	if r.Proto != "" && r.Proto != proto {
		return false
	}
	if r.OrigDPort != 0 {
		return r.OrigDPort == hostPort
	}
	return r.DPort != 0 && r.DPort == containerPort
}
