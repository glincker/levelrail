package main

import (
	"reflect"
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
)

func TestMapCaproverApplication(t *testing.T) {
	tests := []struct {
		name                string
		app                 caproverAppDefinition
		rootDomain          string
		includeSecretValues bool
		check               func(t *testing.T, got mappedApp)
	}{
		{
			name: "exposed app maps default subdomain plus port",
			app: caproverAppDefinition{
				AppName: "my-app", ContainerHTTPPort: 3000,
			},
			rootDomain: "example.com",
			check: func(t *testing.T, got mappedApp) {
				assertBasicMapping(t, got, "my-app", spec.BuildDockerfile, "", []string{"my-app.example.com"}, 3000)
				assertHasIssue(t, got.Issues, "captainDefinitionRelativeFilePath", issueReview, "a review note about the unresolvable build config")
			},
		},
		{
			name: "not exposed as web app omits default subdomain",
			app: caproverAppDefinition{
				AppName: "internal-svc", ContainerHTTPPort: 8080, NotExposeAsWebApp: true,
			},
			rootDomain: "example.com",
			check: func(t *testing.T, got mappedApp) {
				if got.Service.Domains != nil {
					t.Errorf("Domains = %v, want nil when notExposeAsWebApp is set", got.Service.Domains)
				}
				if got.Service.Port != 8080 {
					t.Errorf("Port = %d, want 8080", got.Service.Port)
				}
			},
		},
		{
			name: "custom domains are appended to the default subdomain",
			app: caproverAppDefinition{
				AppName: "web", ContainerHTTPPort: 80,
				CustomDomain: []caproverCustomDomain{{PublicDomain: "app.acme.com", HasSsl: true}, {PublicDomain: "www.acme.com"}},
			},
			rootDomain: "example.com",
			check: func(t *testing.T, got mappedApp) {
				want := []string{"web.example.com", "app.acme.com", "www.acme.com"}
				if !reflect.DeepEqual(got.Service.Domains, want) {
					t.Errorf("Domains = %v, want %v", got.Service.Domains, want)
				}
			},
		},
		{
			name:       "name is sanitized",
			app:        caproverAppDefinition{AppName: "My App", ContainerHTTPPort: 3000},
			rootDomain: "example.com",
			check: func(t *testing.T, got mappedApp) {
				want := sanitizeServiceName("My App")
				if got.ServiceName != want {
					t.Errorf("ServiceName = %q, want %q", got.ServiceName, want)
				}
				assertHasIssue(t, got.Issues, "name", issueReview, "a review note about the sanitized name")
			},
		},
		{
			name: "extra port mappings are dropped",
			app: caproverAppDefinition{
				AppName: "multiport", ContainerHTTPPort: 3000,
				Ports: []caproverPort{{HostPort: 5432, ContainerPort: 5432}},
			},
			rootDomain: "example.com",
			check: func(t *testing.T, got mappedApp) {
				assertHasIssue(t, got.Issues, "ports", issueDropped, "a dropped issue about the extra port mapping")
			},
		},
		{
			name: "named volume is dropped",
			app: caproverAppDefinition{
				AppName: "withdata", ContainerHTTPPort: 3000, HasPersistentData: true,
				Volumes: []caproverVolume{{VolumeName: "withdata-data", ContainerPath: "/data"}},
			},
			rootDomain: "example.com",
			check: func(t *testing.T, got mappedApp) {
				assertHasIssue(t, got.Issues, "volumes", issueDropped, "a dropped issue naming the volume")
				assertNoIssue(t, got.Issues, "volumes", issueBlocking, "no blocking issue for a volume with a Levelrail migration path")
			},
		},
		{
			name: "bind mount is dropped and named by host path",
			app: caproverAppDefinition{
				AppName: "bound", ContainerHTTPPort: 3000,
				Volumes: []caproverVolume{{HostPath: "/host/data", ContainerPath: "/data"}},
			},
			rootDomain: "example.com",
			check: func(t *testing.T, got mappedApp) {
				assertHasIssue(t, got.Issues, "volumes", issueDropped, "a dropped issue naming the bind mount")
			},
		},
		{
			name: "hasPersistentData with no listed volumes gets a review issue",
			app: caproverAppDefinition{
				AppName: "mystery-data", ContainerHTTPPort: 3000, HasPersistentData: true,
			},
			rootDomain: "example.com",
			check: func(t *testing.T, got mappedApp) {
				assertHasIssue(t, got.Issues, "volumes", issueReview, "a review issue about unlisted persistent data")
			},
		},
		{
			name:       "instance count above one gets a review issue",
			app:        caproverAppDefinition{AppName: "scaled", ContainerHTTPPort: 3000, InstanceCount: 3},
			rootDomain: "example.com",
			check: func(t *testing.T, got mappedApp) {
				assertHasIssue(t, got.Issues, "instanceCount", issueReview, "a review issue about the instance count")
			},
		},
		{
			name:       "instance count of one is not flagged",
			app:        caproverAppDefinition{AppName: "single", ContainerHTTPPort: 3000, InstanceCount: 1},
			rootDomain: "example.com",
			check: func(t *testing.T, got mappedApp) {
				assertNoIssue(t, got.Issues, "instanceCount", issueReview, "no issue for a single instance")
			},
		},
		{
			name: "env vars default to key-only secret placeholders",
			app: caproverAppDefinition{
				AppName: "envtest", ContainerHTTPPort: 3000,
				EnvVars: []caproverEnvVar{{Key: "DATABASE_URL", Value: "postgres://real"}, {Key: "API_KEY", Value: "secret"}},
			},
			rootDomain: "example.com",
			check: func(t *testing.T, got mappedApp) {
				assertEnvKeysPlaceholder(t, got, []string{"API_KEY", "DATABASE_URL"}, "envVars")
			},
		},
		{
			name: "env vars with include-secret-values carries real values",
			app: caproverAppDefinition{
				AppName: "envtest", ContainerHTTPPort: 3000,
				EnvVars: []caproverEnvVar{{Key: "PLAIN", Value: "literal-value"}},
			},
			rootDomain:          "example.com",
			includeSecretValues: true,
			check: func(t *testing.T, got mappedApp) {
				assertEnvLiteral(t, got, map[string]string{"PLAIN": "literal-value"})
			},
		},
		{
			name:       "no repo url or ref is ever set",
			app:        caproverAppDefinition{AppName: "web", ContainerHTTPPort: 3000},
			rootDomain: "example.com",
			check: func(t *testing.T, got mappedApp) {
				if got.RepoURL != "" || got.Ref != "" {
					t.Errorf("RepoURL/Ref = %q/%q, want both empty (CapRover exposes no git source)", got.RepoURL, got.Ref)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapCaproverApplication(tt.app, tt.rootDomain, tt.includeSecretValues)
			runMapTestCase(t, false, "", tt.check, got)
		})
	}
}
