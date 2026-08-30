package main

import (
	"reflect"
	"sort"
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
)

func TestMapDokployApplication(t *testing.T) {
	tests := []struct {
		name                string
		app                 dokployApplication
		domains             []dokployDomain
		includeSecretValues bool
		wantBlocking        bool
		wantIssueSubstr     string
		check               func(t *testing.T, got mappedApp)
	}{
		{
			name: "github dockerfile build maps directly",
			app: dokployApplication{
				Name: "My App", SourceType: "github", BuildType: "dockerfile",
				Dockerfile: "/backend/Dockerfile", Owner: "acme", Repository: "my-app", Branch: "main",
			},
			domains: []dokployDomain{{Host: "app.example.com", Port: 3000}},
			check: func(t *testing.T, got mappedApp) {
				assertBasicMapping(t, got, "my-app", spec.BuildDockerfile, "backend/Dockerfile", []string{"app.example.com"}, 3000)
				if got.RepoURL != "https://github.com/acme/my-app.git" {
					t.Errorf("RepoURL = %q, want github URL", got.RepoURL)
				}
				if got.Ref != "main" {
					t.Errorf("Ref = %q, want main", got.Ref)
				}
			},
		},
		{
			name:    "railpack build maps directly",
			app:     dokployApplication{Name: "web", SourceType: "github", BuildType: "railpack"},
			domains: []dokployDomain{{Host: "web.example.com", Port: 8080}},
			check: func(t *testing.T, got mappedApp) {
				if got.Service.BuildType != spec.BuildRailpack {
					t.Errorf("BuildType = %q, want %q", got.Service.BuildType, spec.BuildRailpack)
				}
			},
		},
		{
			name:    "static build drops port",
			app:     dokployApplication{Name: "docs", SourceType: "github", BuildType: "static"},
			domains: []dokployDomain{{Host: "docs.example.com", Port: 80}},
			check: func(t *testing.T, got mappedApp) {
				if got.Service.BuildType != spec.BuildStatic {
					t.Errorf("BuildType = %q, want %q", got.Service.BuildType, spec.BuildStatic)
				}
				if got.Service.Port != 0 {
					t.Errorf("Port = %d, want 0 for a static build", got.Service.Port)
				}
				assertHasIssue(t, got.Issues, "domains", issueDropped, "a dropped domains issue")
			},
		},
		{
			name:            "nixpacks build is blocking",
			app:             dokployApplication{Name: "old-app", SourceType: "github", BuildType: "nixpacks"},
			wantBlocking:    true,
			wantIssueSubstr: "Nixpacks",
		},
		{
			name:            "heroku buildpacks is blocking",
			app:             dokployApplication{Name: "heroku-app", SourceType: "github", BuildType: "heroku_buildpacks"},
			wantBlocking:    true,
			wantIssueSubstr: "heroku_buildpacks",
		},
		{
			name:            "docker source type is blocking",
			app:             dokployApplication{Name: "image-app", SourceType: "docker", DockerImage: "ghcr.io/acme/app:latest"},
			wantBlocking:    true,
			wantIssueSubstr: "prebuilt image",
		},
		{
			name:            "drop source type is blocking",
			app:             dokployApplication{Name: "upload-app", SourceType: "drop"},
			wantBlocking:    true,
			wantIssueSubstr: "file upload",
		},
		{
			name:            "unrecognized build type is blocking",
			app:             dokployApplication{Name: "mystery", SourceType: "github", BuildType: "something-new"},
			wantBlocking:    true,
			wantIssueSubstr: "unrecognized",
		},
		{
			name: "gitlab source leaves repo url blank with a review issue",
			app: dokployApplication{
				Name: "gl-app", SourceType: "gitlab", BuildType: "dockerfile",
				GitlabOwner: "acme", GitlabRepository: "gl-app", GitlabBranch: "main",
			},
			check: func(t *testing.T, got mappedApp) {
				if got.RepoURL != "" {
					t.Errorf("RepoURL = %q, want empty for gitlab (host unknown)", got.RepoURL)
				}
				assertHasIssue(t, got.Issues, "repository", issueReview, "a review issue about the missing GitLab host")
			},
		},
		{
			name: "bitbucket maps directly",
			app: dokployApplication{
				Name: "bb-app", SourceType: "bitbucket", BuildType: "dockerfile",
				BitbucketOwner: "acme", BitbucketRepository: "bb-app", BitbucketBranch: "develop",
			},
			check: func(t *testing.T, got mappedApp) {
				if got.RepoURL != "https://bitbucket.org/acme/bb-app.git" {
					t.Errorf("RepoURL = %q, want bitbucket URL", got.RepoURL)
				}
				if got.Ref != "develop" {
					t.Errorf("Ref = %q, want develop", got.Ref)
				}
			},
		},
		{
			name: "custom git url passes through",
			app: dokployApplication{
				Name: "custom-app", SourceType: "git", BuildType: "dockerfile",
				CustomGitURL: "https://git.internal.example.com/acme/custom-app.git", CustomGitBranch: "trunk",
			},
			check: func(t *testing.T, got mappedApp) {
				if got.RepoURL != "https://git.internal.example.com/acme/custom-app.git" {
					t.Errorf("RepoURL = %q, want the custom git url unchanged", got.RepoURL)
				}
				if got.Ref != "trunk" {
					t.Errorf("Ref = %q, want trunk", got.Ref)
				}
			},
		},
		{
			name:    "multiple domains, differing port on second is a review issue",
			app:     dokployApplication{Name: "multi", SourceType: "github", BuildType: "dockerfile"},
			domains: []dokployDomain{{Host: "a.example.com", Port: 3000}, {Host: "b.example.com", Port: 4000}},
			check: func(t *testing.T, got mappedApp) {
				want := []string{"a.example.com", "b.example.com"}
				if !reflect.DeepEqual(got.Service.Domains, want) {
					t.Errorf("Domains = %v, want %v", got.Service.Domains, want)
				}
				if got.Service.Port != 3000 {
					t.Errorf("Port = %d, want 3000 (first domain)", got.Service.Port)
				}
				assertHasIssue(t, got.Issues, "domains", issueReview, "a review issue about the differing port")
			},
		},
		{
			name: "memory in whole gibibytes",
			app:  dokployApplication{Name: "big", SourceType: "github", BuildType: "dockerfile", MemoryLimit: "2147483648"},
			check: func(t *testing.T, got mappedApp) {
				assertMemory(t, got, "2Gi")
			},
		},
		{
			name: "memory zero is omitted",
			app:  dokployApplication{Name: "unlimited", SourceType: "github", BuildType: "dockerfile", MemoryLimit: "0"},
			check: func(t *testing.T, got mappedApp) {
				assertMemory(t, got, "")
			},
		},
		{
			name: "cpu in nanocpus converts to cores",
			app:  dokployApplication{Name: "cpubound", SourceType: "github", BuildType: "dockerfile", CPULimit: "1500000000"},
			check: func(t *testing.T, got mappedApp) {
				assertCPU(t, got, 1.5)
			},
		},
		{
			name: "unparseable cpu leaves it unset with a review issue",
			app:  dokployApplication{Name: "badcpu", SourceType: "github", BuildType: "dockerfile", CPULimit: "1.5"},
			check: func(t *testing.T, got mappedApp) {
				assertCPU(t, got, 0)
				assertHasIssue(t, got.Issues, "cpuLimit", issueReview, "a review issue about the unparseable cpuLimit")
			},
		},
		{
			name: "negative parsed memory leaves it unset with a review issue",
			app:  dokployApplication{Name: "negmem", SourceType: "github", BuildType: "dockerfile", MemoryLimit: "-1"},
			check: func(t *testing.T, got mappedApp) {
				assertMemory(t, got, "")
				assertHasIssue(t, got.Issues, "memoryLimit", issueReview, "a review issue about the negative memoryLimit")
			},
		},
		{
			name: "negative parsed cpu leaves it unset with a review issue",
			app:  dokployApplication{Name: "negcpu", SourceType: "github", BuildType: "dockerfile", CPULimit: "-1"},
			check: func(t *testing.T, got mappedApp) {
				assertCPU(t, got, 0)
				assertHasIssue(t, got.Issues, "cpuLimit", issueReview, "a review issue about the negative cpuLimit")
			},
		},
		{
			name: "env vars default to key-only secret placeholders",
			app:  dokployApplication{Name: "envtest", SourceType: "github", BuildType: "dockerfile", Env: "DATABASE_URL=postgres://real\nAPI_KEY=secret\n"},
			check: func(t *testing.T, got mappedApp) {
				assertEnvKeysPlaceholder(t, got, []string{"API_KEY", "DATABASE_URL"}, "env")
			},
		},
		{
			name:                "env vars with include-secret-values carries real values",
			app:                 dokployApplication{Name: "envtest", SourceType: "github", BuildType: "dockerfile", Env: "PLAIN=literal-value\n# a comment\n\nQUOTED=\"quoted-value\""},
			includeSecretValues: true,
			check: func(t *testing.T, got mappedApp) {
				assertEnvLiteral(t, got, map[string]string{"PLAIN": "literal-value", "QUOTED": "quoted-value"})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapDokployApplication(tt.app, tt.domains, tt.includeSecretValues)
			runMapTestCase(t, tt.wantBlocking, tt.wantIssueSubstr, tt.check, got)
		})
	}
}

func TestParseDokployEnv(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want map[string]string
	}{
		{"empty", "", map[string]string{}},
		{"simple", "A=1\nB=2", map[string]string{"A": "1", "B": "2"}},
		{"skips comments and blanks", "# comment\n\nA=1\n  \nB=2", map[string]string{"A": "1", "B": "2"}},
		{"strips surrounding quotes", `A="quoted"` + "\n" + `B='single'`, map[string]string{"A": "quoted", "B": "single"}},
		{"value containing equals sign", "URL=postgres://host/db?opt=1&other=2", map[string]string{"URL": "postgres://host/db?opt=1&other=2"}},
		{"malformed line without equals is skipped", "NOTANASSIGNMENT\nA=1", map[string]string{"A": "1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDokployEnv(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseDokployEnv(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestMapDokployEnvKeysSorted(t *testing.T) {
	keys, _, _ := mapDokployEnv("ZEBRA=1\nALPHA=2", false, nil)
	if !sort.StringsAreSorted(keys) {
		t.Errorf("keys = %v, want sorted", keys)
	}
}
