// Package agentupdateguard fails early when a release change would silently
// break agent self-update: asset names, checksums and signature assets, the
// signer identity, the --version contract and the UpdateCheck wire shape.
package agentupdateguard

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/cliupdate"
	"github.com/glincker/levelrail/internal/selfupgrade"
)

// selfUpdatePlatforms are the agent platforms self-update asks assets for.
var selfUpdatePlatforms = [][2]string{{"linux", "amd64"}, {"linux", "arm64"}, {"darwin", "amd64"}, {"darwin", "arm64"}}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(t), filepath.FromSlash(rel))) //nolint:gosec // fixed repo-relative guard inputs
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// agentJob returns the build-agent-binaries job text of the release workflow.
func agentJob(t *testing.T, wf string) string {
	t.Helper()
	i := strings.Index(wf, "  build-agent-binaries:")
	if i < 0 {
		t.Fatal("release.yml has no build-agent-binaries job; self-update would have no assets")
	}
	rest := wf[i+1:]
	if j := regexp.MustCompile(`\n  [a-z-]+:\n`).FindStringIndex(rest); j != nil {
		return rest[:j[0]]
	}
	return rest
}

func TestReleaseBuildsEveryAssetSelfUpdateAsks(t *testing.T) {
	job := agentJob(t, readRepoFile(t, ".github/workflows/release.yml"))
	list := func(key string) []string {
		m := regexp.MustCompile(key + `: \[([^\]]*)\]`).FindStringSubmatch(job)
		if m == nil {
			t.Fatalf("agent matrix has no %s list", key)
		}
		return strings.Split(strings.ReplaceAll(m[1], " ", ""), ",")
	}
	oses, arches := list("goos"), list("goarch")
	has := func(set []string, v string) bool {
		for _, s := range set {
			if s == v {
				return true
			}
		}
		return false
	}
	for _, p := range selfUpdatePlatforms {
		if !has(oses, p[0]) || !has(arches, p[1]) {
			t.Errorf("release does not build %s", cliupdate.Asset("levelrail-agent", p[0], p[1]))
		}
	}
	want := cliupdate.Asset("levelrail-agent", "${{ matrix.goos }}", "${{ matrix.goarch }}")
	if !strings.Contains(job, `-o "`+want+`"`) {
		t.Errorf("agent build output is not named %s; agents would ask for assets that do not exist", want)
	}
	if !strings.Contains(job, "internal/version.Version=${{ env.RELEASE_TAG }}") {
		t.Error("agent build does not stamp the release tag; the smoke check compares it with the target")
	}
}

func TestReleaseSignsAndListsChecksums(t *testing.T) {
	wf := readRepoFile(t, ".github/workflows/release.yml")
	for _, need := range []string{"sha256sum -- * > checksums.txt", "--bundle checksums.txt.sigstore.json"} {
		if !strings.Contains(wf, need) {
			t.Errorf("release.yml lost %q; agents verify checksums.txt and its sigstore bundle", need)
		}
	}
}

func TestSignerIdentityMatchesReleaseWorkflowRef(t *testing.T) {
	const repo = "GLINCKER/levelrail"
	re := regexp.MustCompile(selfupgrade.ReleaseIdentityRegexp(repo))
	if !re.MatchString("https://github.com/" + repo + "/.github/workflows/release.yml@refs/tags/v1.2.3") {
		t.Fatal("identity regexp rejects the release workflow tag ref")
	}
	for _, bad := range []string{
		"https://github.com/" + repo + "/.github/workflows/other.yml@refs/tags/v1.2.3",
		"https://github.com/" + repo + "/.github/workflows/release.yml@refs/heads/main",
	} {
		if re.MatchString(bad) {
			t.Errorf("identity regexp accepts %s", bad)
		}
	}
	wf := readRepoFile(t, ".github/workflows/release.yml")
	if !regexp.MustCompile(`(?m)^\s+tags:\s*\n\s+- ['"]?v\*`).MatchString(wf) && !strings.Contains(wf, "ref_type == 'tag'") {
		t.Error("release.yml no longer runs from a v* tag, so signatures would not match the pinned identity")
	}
}

func TestAgentVersionOutputIsTheBareTag(t *testing.T) {
	main := readRepoFile(t, "cmd/levelrail-agent/main.go")
	if !strings.Contains(main, `case "--version"`) || !strings.Contains(main, "Fprintln(out, version.Version)") {
		t.Fatal("levelrail-agent --version must print version.Version alone; the smoke check compares it with the target")
	}
}

// Wire compatibility is additive only: an old agent must keep reading an
// answer from a newer control plane, so these numbers and types never change.
func TestUpdateCheckProtoStaysBackwardCompatible(t *testing.T) {
	proto := readRepoFile(t, "proto/agent/v1/agent.proto")
	m := regexp.MustCompile(`(?s)message UpdateCheckResponse \{(.*?)\n\}`).FindStringSubmatch(proto)
	if m == nil {
		t.Fatal("UpdateCheckResponse is missing")
	}
	fields := map[string]string{
		"action": "string 1", "channel": "string 2", "target_version": "string 3", "reason": "string 4", "auto_apply": "bool 5",
	}
	for name, spec := range fields {
		parts := strings.Fields(spec)
		if !regexp.MustCompile(`(?m)^\s*` + parts[0] + `\s+` + name + `\s*=\s*` + parts[1] + `;`).MatchString(m[1]) {
			t.Errorf("UpdateCheckResponse.%s must stay %s = %s", name, parts[0], parts[1])
		}
	}
	if !strings.Contains(proto, "rpc UpdateCheck(UpdateCheckRequest) returns (UpdateCheckResponse);") {
		t.Error("the UpdateCheck rpc signature changed")
	}
}
