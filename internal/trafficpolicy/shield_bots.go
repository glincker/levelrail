package trafficpolicy

import (
	_ "embed"
	"fmt"
	"os"
	"strings"
)

//go:embed shield_bots.txt
var shippedBotList string

// ParseBotList reads one substring per line, skipping blanks and # comments.
func ParseBotList(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.ToLower(strings.TrimSpace(line))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// DefaultBotAgents returns the shipped denylist.
func DefaultBotAgents() []string { return ParseBotList(shippedBotList) }

// BotMatcher matches user agents against lower-case substrings.
type BotMatcher struct{ patterns []string }

// BotAgentsFor returns the shipped denylist, or the file named by
// l.ShieldBotListFile when set.
func BotAgentsFor(l Limits) ([]string, error) {
	if l.ShieldBotListFile == "" {
		return DefaultBotAgents(), nil
	}
	data, err := os.ReadFile(l.ShieldBotListFile)
	if err != nil {
		return nil, fmt.Errorf("trafficpolicy: read bot list: %w", err)
	}
	return ParseBotList(string(data)), nil
}

// NewBotMatcher builds a matcher over BotAgentsFor plus extra per-domain entries.
func NewBotMatcher(l Limits, extra []string) (*BotMatcher, error) {
	base, err := BotAgentsFor(l)
	if err != nil {
		return nil, err
	}
	for _, e := range extra {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			base = append(base, e)
		}
	}
	return &BotMatcher{patterns: base}, nil
}

// Match reports whether userAgent contains a denylisted substring.
func (m *BotMatcher) Match(userAgent string) bool {
	if m == nil || userAgent == "" {
		return false
	}
	ua := strings.ToLower(userAgent)
	for _, p := range m.patterns {
		if strings.Contains(ua, p) {
			return true
		}
	}
	return false
}
