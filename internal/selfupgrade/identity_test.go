package selfupgrade

import (
	"regexp"
	"testing"
)

func TestReleaseIdentityRegexpPinsVersionTags(t *testing.T) {
	re := regexp.MustCompile(ReleaseIdentityRegexp("GLINCKER/levelrail"))
	base := "https://github.com/glincker/levelrail/.github/workflows/release.yml@"
	if !re.MatchString(base + "refs/tags/v1.5.8") {
		t.Fatal("tag identity must match")
	}
	for _, ref := range []string{"refs/heads/main", "refs/heads/v1", "refs/tags/nightly"} {
		if re.MatchString(base + ref) {
			t.Fatalf("%s must not match", ref)
		}
	}
}
