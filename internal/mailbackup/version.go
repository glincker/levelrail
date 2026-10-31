package mailbackup

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var versionRe = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:$|[-+])`)

type semver [3]int

// parseVersion reads mox's own version from the first token of `mox version`
// output ("v0.0.17-go1.27.0"); a bare Go version or "(devel)" is refused.
func parseVersion(out string) (semver, string, error) {
	tok := ""
	if f := strings.Fields(out); len(f) > 0 {
		tok = f[0]
	}
	var m []string
	if !strings.HasPrefix(tok, "go") {
		m = versionRe.FindStringSubmatch(tok)
	}
	if m == nil {
		return semver{}, "", fmt.Errorf("cannot parse mox version from %q", out)
	}
	var v semver
	for i := 0; i < 3; i++ {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return semver{}, "", err
		}
		v[i] = n
	}
	return v, fmt.Sprintf("v%d.%d.%d", v[0], v[1], v[2]), nil
}

func (a semver) less(b semver) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}
