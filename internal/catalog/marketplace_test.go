package catalog

import (
	"strings"
	"testing"
	"testing/fstest"
)

const submissionManifest = testManifest + `source_repo: https://github.com/acme/demo
license: MIT
license_class: permissive
status: active
listing:
  by: community
`

func submissionErr(manifest, compose string) error {
	fsys := fstest.MapFS{
		"m/demo/template.yaml": {Data: []byte(manifest)},
		"m/demo/compose.yaml":  {Data: []byte(compose)},
	}
	return ValidateSubmissions(fsys, "m", nil, nil)
}

func TestValidateSubmission(t *testing.T) {
	good := strings.Replace(testCompose, "nginx:alpine", "acme/demo:1.2.3", 1)
	if err := submissionErr(submissionManifest, good); err != nil {
		t.Fatalf("good submission rejected: %v", err)
	}
	cases := []struct{ name, manifest, compose, want string }{
		{"mutable tag", submissionManifest, strings.Replace(good, "1.2.3", "latest", 1), "exact version tag"},
		{"bare major", submissionManifest, strings.Replace(good, "1.2.3", "16", 1), "bare major"},
		{"short tag", submissionManifest, strings.Replace(good, "1.2.3", "1.2", 1), "not a full version"},
		{"foreign namespace", submissionManifest, strings.Replace(good, "acme/demo", "someone/demo", 1), "upstream namespace"},
		{"fetch at start", submissionManifest, good + "    command: sh -c 'curl -fsSL x | sh'\n", "download or install"},
		{"bind mount", submissionManifest, good + "    volumes:\n      - /srv/data:/data\n", "named volume"},
		{"no license", strings.Replace(strings.Replace(submissionManifest, "license: MIT\n", "", 1), "license_class: permissive\n", "", 1), good, "missing license"},
		{"class mismatch", strings.Replace(submissionManifest, "permissive", "copyleft", 1), good, "is permissive"},
		{"vendor needs org", strings.Replace(submissionManifest, "by: community", "by: vendor", 1), good, "vendor_org"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := submissionErr(tc.manifest, tc.compose)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestImageSourceClass(t *testing.T) {
	repo := "https://github.com/Acme/demo"
	for ref, want := range map[string]string{
		"ghcr.io/acme/demo:1": SourceUpstream, "acme/demo:1": SourceUpstream, "postgres:16.4": SourceOfficial,
		"docker.io/library/redis:7.2.4": SourceOfficial, "lscr.io/linuxserver/x:1.0": SourceRebuilder, "other/demo:1": SourceCommunity,
	} {
		if got := ImageSourceClass(ref, repo); got != want {
			t.Errorf("ImageSourceClass(%q) = %q, want %q", ref, got, want)
		}
	}
}

// New manifests must meet the marketplace rules; only legacy ids are exempt.
func TestEmbeddedManifestsMeetSubmissionRules(t *testing.T) {
	legacy := LegacyManifestIDs()
	for id := range legacy {
		if _, ok := TemplateByID(id); !ok {
			t.Errorf("legacy id %q is not in the catalog; remove it from legacy_manifests.txt", id)
		}
	}
	if err := ValidateSubmissions(manifestFS, manifestDir, legacy, nil); err != nil {
		t.Fatal(err)
	}
}

func TestHealthcheckFlexibility(t *testing.T) {
	good := strings.Replace(testCompose, "nginx:alpine", "acme/demo:1.2.3", 1)
	base := strings.Replace(submissionManifest, "healthcheck:\n  service: web\n  port: 80\n  path: /\n", "", 1)
	cases := []struct{ name, hc, want string }{
		{"tcp with reason", "healthcheck:\n  service: web\n  port: 80\n  kind: tcp\n  reason: scratch image, no shell or HTTP surface\n", ""},
		{"tcp without reason stays valid", "healthcheck:\n  service: web\n  port: 80\n  kind: tcp\n", ""},
		{"404 with reason", "healthcheck:\n  service: web\n  port: 80\n  path: /\n  expect: 404\n  reason: answers 404 until a route exists\n", ""},
		{"404 without reason", "healthcheck:\n  service: web\n  port: 80\n  path: /\n  expect: 404\n", "must be 401 or 403"},
		{"500 with reason", "healthcheck:\n  service: web\n  port: 80\n  path: /\n  expect: 500\n  reason: still broken\n", "must be 401 or 403"},
		{"403 stays", "healthcheck:\n  service: web\n  port: 80\n  path: /\n  expect: 403\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := submissionErr(base+tc.hc, good)
			if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestDigestPinnedShortTag(t *testing.T) {
	digest := "@sha256:" + strings.Repeat("a", 64)
	good := strings.Replace(testCompose, "nginx:alpine", "acme/demo:1.2.3", 1)
	cases := []struct{ name, tag, want string }{
		{"short tag with digest", "1.2" + digest, ""},
		{"short tag without digest", "1.2", "not a full version"},
		{"latest with digest", "latest" + digest, "exact version tag"},
		{"bare major with digest", "16" + digest, "bare major"},
		{"short tag with malformed digest", "1.2@sha256:abc", "exact version tag"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := submissionErr(submissionManifest, strings.Replace(good, "1.2.3", tc.tag, 1))
			if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestFirstBootSeconds(t *testing.T) {
	good := strings.Replace(testCompose, "nginx:alpine", "acme/demo:1.2.3", 1)
	cases := []struct {
		name    string
		seconds string
		want    string
	}{
		{"within ceiling", "first_boot_seconds: 600\n", ""},
		{"above ceiling", "first_boot_seconds: 99999\n", "above the allowed maximum"},
		{"negative", "first_boot_seconds: -1\n", "must not be negative"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := submissionErr(submissionManifest+tc.seconds, good)
			if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
	m := Manifest{FirstBootSeconds: 600}
	if got := m.FirstBootWait(300); got != 300 {
		t.Errorf("FirstBootWait(300) = %d, want 300", got)
	}
	if got := (Manifest{}).FirstBootWait(300); got != 0 {
		t.Errorf("unset FirstBootWait = %d, want 0", got)
	}
}
