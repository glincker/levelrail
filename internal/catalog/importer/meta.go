package importer

import (
	"strings"
)

var categoryMap = map[string]string{
	"productivity": "Productivity", "documentation": "Productivity", "rss": "Productivity",
	"devtools": "Developer Tools", "development": "Developer Tools", "developer-tools": "Developer Tools",
	"git": "Developer Tools", "ci": "Developer Tools", "backend": "Developer Tools", "api": "Developer Tools",
	"mcp":   "Developer Tools",
	"media": "Media", "games": "Media",
	"cms": "Applications", "search": "Applications", "helpdesk": "Applications", "health": "Applications",
	"family": "Applications",
	"ai":     "AI", "storage": "Storage", "monitoring": "Monitoring", "automation": "Automation",
	"analytics": "Analytics",
	"messaging": "Communication", "communication": "Communication", "email": "Communication", "mail": "Communication",
	"auth": "Security", "security": "Security", "vpn": "Security",
	"database": "Database Tools", "databases": "Database Tools",
	"finance": "Finance", "proxy": "Infrastructure", "networking": "Infrastructure",
}

var categoryIcons = map[string]string{
	"AI": "sparkle", "Analytics": "chart-bar", "Applications": "app-window", "Automation": "lightning",
	"Communication": "chat-circle", "Dashboard": "squares-four", "Database Tools": "database",
	"Developer Tools": "code", "Finance": "coins", "Infrastructure": "stack", "Media": "play-circle",
	"Monitoring": "chart-line", "Productivity": "check-square", "Security": "shield-check", "Storage": "hard-drives",
}

func mapCategory(raw string) string {
	first, _, _ := strings.Cut(raw, ",")
	if c, ok := categoryMap[strings.ToLower(strings.TrimSpace(first))]; ok {
		return c
	}
	return "Applications"
}

func iconFor(category string) string {
	if i, ok := categoryIcons[category]; ok {
		return i
	}
	return "package"
}

// plainText trims s and replaces em and en dashes with commas, which
// the repository's writing rules require everywhere.
func plainText(s string) string {
	em, en := string(rune(0x2014)), string(rune(0x2013))
	r := strings.NewReplacer(" "+em+" ", ", ", em, ", ", " "+en+" ", ", ", en, ", ")
	return strings.TrimSpace(r.Replace(s))
}

const mib = 1 << 20

var memoryByImage = []struct {
	match string
	mib   int64
}{
	{"elasticsearch", 1024}, {"opensearch", 1024}, {"clickhouse", 1024}, {"mongo", 512},
	{"mysql", 512}, {"mariadb", 256}, {"postgres", 256}, {"redis", 128}, {"valkey", 128},
	{"keydb", 128}, {"memcached", 64},
}

// estimateMemory is a static advisory: a per-image guess summed over
// the stack, never enforced.
func estimateMemory(svcs map[string]*outService) int64 {
	var total int64
	for _, s := range svcs {
		name := strings.ToLower(s.Image)
		added := int64(256)
		for _, m := range memoryByImage {
			if strings.Contains(name, m.match) {
				added = m.mib
				break
			}
		}
		total += added
	}
	if total < 256 {
		total = 256
	}
	return total * mib
}
