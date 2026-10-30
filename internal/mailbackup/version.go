package mailbackup

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var versionRe = regexp.MustCompile(`v?(\d+)\.(\d+)\.(\d+)`)

type semver [3]int

// parseVersion pulls the first x.y.z out of `mox version` output.
func parseVersion(out string) (semver, string, error) {
	m := versionRe.FindStringSubmatch(strings.TrimSpace(out))
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
