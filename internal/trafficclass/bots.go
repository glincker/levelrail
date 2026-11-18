package trafficclass

import (
	_ "embed"
	"strings"
)

//go:embed bots.txt
var botTokens string

// maxBotAgent bounds how much of the User-Agent is scanned, matching what the access ring keeps.
const maxBotAgent = 160

// automaton is an Aho-Corasick matcher over lowercase tokens. Matching folds
// ASCII case on the fly, so scanning a User-Agent allocates nothing.
type automaton struct {
	root  [256]int32
	edges [][]edge
	fail  []int32
	// name is the token index that ends at, or is a suffix of, each state; -1 when none.
	name   []int32
	tokens []string
}

type edge struct {
	b  byte
	to int32
}

func newAutomaton(tokens []string) *automaton {
	a := &automaton{tokens: tokens, edges: [][]edge{nil}, fail: []int32{0}, name: []int32{-1}}
	for i, tok := range tokens {
		s := int32(0)
		for j := 0; j < len(tok); j++ {
			b := tok[j]
			next := a.step(s, b)
			if next < 0 {
				next = int32(len(a.edges)) //nolint:gosec // bounded by the embedded token list
				a.edges = append(a.edges, nil)
				a.fail = append(a.fail, 0)
				a.name = append(a.name, -1)
				if s == 0 {
					a.root[b] = next
				} else {
					a.edges[s] = append(a.edges[s], edge{b, next})
				}
			}
			s = next
		}
		if a.name[s] < 0 {
			a.name[s] = int32(i)
		}
	}
	a.link()
	return a
}

// step follows an existing trie edge, or returns -1.
func (a *automaton) step(s int32, b byte) int32 {
	if s == 0 {
		if t := a.root[b]; t != 0 {
			return t
		}
		return -1
	}
	for _, e := range a.edges[s] {
		if e.b == b {
			return e.to
		}
	}
	return -1
}

// link computes failure links breadth first and carries the first token name down each chain.
func (a *automaton) link() {
	var queue []int32
	for b := 0; b < 256; b++ {
		if t := a.root[b]; t != 0 {
			queue = append(queue, t)
		}
	}
	for len(queue) > 0 {
		s := queue[0]
		queue = queue[1:]
		for _, e := range a.edges[s] {
			f := a.fail[s]
			for {
				if t := a.step(f, e.b); t >= 0 {
					a.fail[e.to] = t
					break
				}
				if f == 0 {
					break
				}
				f = a.fail[f]
			}
			if a.name[e.to] < 0 {
				a.name[e.to] = a.name[a.fail[e.to]]
			}
			queue = append(queue, e.to)
		}
	}
}

// find returns the first token found in s, ignoring ASCII case.
func (a *automaton) find(s string) (string, bool) {
	state := int32(0)
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b >= 'A' && b <= 'Z' {
			b += 'a' - 'A'
		}
		for {
			if t := a.step(state, b); t >= 0 {
				state = t
				break
			}
			if state == 0 {
				break
			}
			state = a.fail[state]
		}
		if n := a.name[state]; n >= 0 {
			return a.tokens[n], true
		}
	}
	return "", false
}

var botMatcher = newAutomaton(parseTokens(botTokens))

func parseTokens(raw string) []string {
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.ToLower(strings.TrimSpace(line))
		if line != "" && !strings.HasPrefix(line, "#") {
			out = append(out, line)
		}
	}
	return out
}

// botName reports whether the client is automated: the shield turned it away,
// it sent no User-Agent, it is a headless browser, or it matches a known token.
func botName(ua string, shielded bool) (string, bool) {
	if len(ua) > maxBotAgent {
		ua = ua[:maxBotAgent]
	}
	if ua == "" {
		return UnknownBot, true
	}
	if name, ok := botMatcher.find(ua); ok {
		return name, true
	}
	if shielded {
		return UnknownBot, true
	}
	return "", false
}
