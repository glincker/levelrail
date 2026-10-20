package main

import (
	"reflect"
	"strings"
	"testing"
)

// runMapTestCase runs the assertions shared by TestMapCoolifyApplication and
// TestMapDokployApplication: both tables share this exact check sequence,
// only the mapping call that produces got differs per provider.
func runMapTestCase(t *testing.T, wantBlocking bool, wantIssueSubstr string, check func(t *testing.T, got mappedApp), got mappedApp) {
	t.Helper()
	if got.Blocking != wantBlocking {
		t.Fatalf("Blocking = %v, want %v (issues=%+v)", got.Blocking, wantBlocking, got.Issues)
	}
	if wantIssueSubstr != "" {
		found := false
		for _, iss := range got.Issues {
			if strings.Contains(iss.Detail, wantIssueSubstr) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Issues = %+v, want one containing %q", got.Issues, wantIssueSubstr)
		}
	}
	if check != nil {
		check(t, got)
	}
}

func assertHasIssue(t *testing.T, issues []migrationIssue, field string, severity issueSeverity, wantMsg string) {
	t.Helper()
	if !hasIssue(issues, field, severity) {
		t.Errorf("Issues = %+v, want %s", issues, wantMsg)
	}
}

func assertNoIssue(t *testing.T, issues []migrationIssue, field string, severity issueSeverity, wantMsg string) {
	t.Helper()
	if hasIssue(issues, field, severity) {
		t.Errorf("Issues = %+v, want %s", issues, wantMsg)
	}
}

func assertMemory(t *testing.T, got mappedApp, want string) {
	t.Helper()
	if got.Service.Memory != want {
		t.Errorf("Memory = %q, want %q", got.Service.Memory, want)
	}
}

func assertCPU(t *testing.T, got mappedApp, want float64) {
	t.Helper()
	if got.Service.CPU != want {
		t.Errorf("CPU = %v, want %v", got.Service.CPU, want)
	}
}

// assertBasicMapping covers the Blocking/ServiceName/BuildType/BuildPath/
// Domains/Port checks common to both providers' "maps directly" test cases.
func assertBasicMapping(t *testing.T, got mappedApp, wantServiceName, wantBuildType, wantBuildPath string, wantDomains []string, wantPort int) {
	t.Helper()
	if got.Blocking {
		t.Fatalf("Blocking = true, want false")
	}
	if got.ServiceName != wantServiceName {
		t.Errorf("ServiceName = %q, want %q", got.ServiceName, wantServiceName)
	}
	if got.Service.BuildType != wantBuildType {
		t.Errorf("BuildType = %q, want %q", got.Service.BuildType, wantBuildType)
	}
	if got.Service.BuildPath != wantBuildPath {
		t.Errorf("BuildPath = %q, want %q", got.Service.BuildPath, wantBuildPath)
	}
	if !reflect.DeepEqual(got.Service.Domains, wantDomains) {
		t.Errorf("Domains = %v, want %v", got.Service.Domains, wantDomains)
	}
	if got.Service.Port != wantPort {
		t.Errorf("Port = %d, want %d", got.Service.Port, wantPort)
	}
}

// assertEnvKeysPlaceholder covers the "env vars default to key-only secret
// placeholders" shape shared by both providers: keys carried, no literal
// values, and a review issue on issueField noting the values weren't fetched.
func assertEnvKeysPlaceholder(t *testing.T, got mappedApp, wantKeys []string, issueField string) {
	t.Helper()
	if !reflect.DeepEqual(got.Service.EnvKeys, wantKeys) {
		t.Errorf("EnvKeys = %v, want %v", got.Service.EnvKeys, wantKeys)
	}
	if len(got.Service.EnvLiteral) != 0 {
		t.Errorf("EnvLiteral = %v, want empty when --include-secret-values was not passed", got.Service.EnvLiteral)
	}
	assertHasIssue(t, got.Issues, issueField, issueReview, "a review note about unfetched values")
}

func assertEnvLiteral(t *testing.T, got mappedApp, want map[string]string) {
	t.Helper()
	if !reflect.DeepEqual(got.Service.EnvLiteral, want) {
		t.Errorf("EnvLiteral = %v, want %v", got.Service.EnvLiteral, want)
	}
}

func ptr[T any](v T) *T { return &v }
