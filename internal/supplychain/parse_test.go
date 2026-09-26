package supplychain

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name)) //nolint:gosec // fixed testdata directory
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func TestParseSBOM(t *testing.T) {
	tests := []struct {
		name       string
		fixture    string
		wantFormat string
		wantCount  int
		wantPkg    Package
	}{
		{"spdx skips the document root", "spdx.json", FormatSPDX, 4, Package{Name: "busybox", Version: "1.36.1-r5", Type: "apk", License: "GPL-2.0-only"}},
		{"cyclonedx skips nameless components", "cyclonedx.json", FormatCycloneDX, 4, Package{Name: "openssl", Version: "3.1.4", Type: "apk", License: "Apache-2.0"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			format, pkgs, err := ParseSBOM(fixture(t, tc.fixture))
			if err != nil {
				t.Fatal(err)
			}
			if format != tc.wantFormat || len(pkgs) != tc.wantCount {
				t.Fatalf("format %q count %d, want %q %d", format, len(pkgs), tc.wantFormat, tc.wantCount)
			}
			found := false
			for _, p := range pkgs {
				found = found || p == tc.wantPkg
			}
			if !found {
				t.Errorf("package %+v not in %+v", tc.wantPkg, pkgs)
			}
		})
	}
}

func TestParseSBOM_Errors(t *testing.T) {
	for name, data := range map[string]string{"not json": "nope", "unknown document": `{"hello":"world"}`} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := ParseSBOM([]byte(data)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestSummarize(t *testing.T) {
	format, pkgs, err := ParseSBOM(fixture(t, "spdx.json"))
	if err != nil {
		t.Fatal(err)
	}
	sum := Summarize(format, pkgs)
	if sum.PackageCount != 4 || sum.Unlicensed != 1 {
		t.Fatalf("summary = %+v", sum)
	}
	if len(sum.Types) == 0 || sum.Types[0] != (TypeCount{Type: "apk", Count: 2}) {
		t.Errorf("types = %+v", sum.Types)
	}
	if len(sum.Licenses) != 2 || sum.Licenses[0] != (LicenseCount{License: "MIT", Count: 2}) {
		t.Errorf("licenses = %+v", sum.Licenses)
	}
	if sum.TopPackages[0].Name != "busybox" {
		t.Errorf("top packages not sorted by name: %+v", sum.TopPackages)
	}
}

func TestSummarize_BoundsTopLists(t *testing.T) {
	var pkgs []Package
	for i := 0; i < 30; i++ {
		pkgs = append(pkgs, Package{Name: string(rune('a' + i%26)), License: string(rune('A' + i%20))})
	}
	sum := Summarize(FormatSPDX, pkgs)
	if len(sum.TopPackages) != topPackageLimit || len(sum.Licenses) != topLicenseLimit {
		t.Errorf("top %d licenses %d", len(sum.TopPackages), len(sum.Licenses))
	}
}

func TestParseTrivy(t *testing.T) {
	sum, err := ParseTrivy(fixture(t, "trivy.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := Counts{Critical: 2, High: 1, Medium: 1, Low: 1, Unknown: 1}
	if sum.Counts != want || sum.Fixable != 3 || sum.Scanner != ScannerTrivy {
		t.Fatalf("summary = %+v", sum)
	}
	if sum.Top[0].ID != "CVE-2024-0001" || sum.Top[0].FixedVersion != "3.1.5" {
		t.Errorf("fixable critical must lead: %+v", sum.Top[0])
	}
	if sum.Top[1].ID != "CVE-2024-0002" {
		t.Errorf("unfixable critical second: %+v", sum.Top[1])
	}
}

func TestParseGrype(t *testing.T) {
	sum, err := ParseGrype(fixture(t, "grype.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := Counts{Critical: 1, High: 1, Low: 1, Unknown: 1}
	if sum.Counts != want || sum.Fixable != 2 || sum.Scanner != ScannerGrype {
		t.Fatalf("summary = %+v", sum)
	}
	if sum.Top[0].Title != "openssl: remote code execution" {
		t.Errorf("title keeps the first line only: %q", sum.Top[0].Title)
	}
}

func TestParseScannerOutput_Malformed(t *testing.T) {
	if _, err := ParseTrivy([]byte("{")); err == nil {
		t.Error("trivy: expected an error")
	}
	if _, err := ParseGrype([]byte("{")); err == nil {
		t.Error("grype: expected an error")
	}
}

func TestSummarizeVulns_TopIsBounded(t *testing.T) {
	var vulns []Vuln
	for i := 0; i < 25; i++ {
		vulns = append(vulns, Vuln{ID: string(rune('A' + i)), Package: "p", Severity: SeverityHigh})
	}
	if got := summarizeVulns(ScannerTrivy, vulns); len(got.Top) != topVulnLimit || got.Counts.High != 25 {
		t.Errorf("top %d high %d", len(got.Top), got.Counts.High)
	}
}

func TestDecide(t *testing.T) {
	crit := &ScanSummary{Counts: Counts{Critical: 2, High: 1}, Fixable: 1}
	clean := &ScanSummary{Counts: Counts{High: 4}}
	tests := []struct {
		name     string
		mode     GateMode
		scan     *ScanSummary
		override string
		want     string
	}{
		{"off never acts", GateOff, crit, "", ActionAllow},
		{"empty mode is off", "", crit, "", ActionAllow},
		{"warn with criticals", GateWarn, crit, "", ActionWarn},
		{"warn without criticals", GateWarn, clean, "", ActionAllow},
		{"block with criticals", GateBlockOnCritical, crit, "", ActionBlock},
		{"block without criticals", GateBlockOnCritical, clean, "", ActionAllow},
		{"block with override", GateBlockOnCritical, crit, "hotfix for outage", ActionOverride},
		{"override ignored when clean", GateBlockOnCritical, clean, "hotfix", ActionAllow},
		{"block fails open without a scan", GateBlockOnCritical, nil, "", ActionAllow},
		{"warn fails open without a scan", GateWarn, nil, "", ActionAllow},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := Decide(tc.mode, tc.scan, tc.override)
			if d.Action != tc.want {
				t.Fatalf("action %q (%s), want %q", d.Action, d.Reason, tc.want)
			}
			if d.Blocked() != (tc.want == ActionBlock) {
				t.Errorf("Blocked() = %v", d.Blocked())
			}
			if tc.want != ActionAllow && d.Reason == "" {
				t.Error("a non-allow decision must carry a reason")
			}
		})
	}
}

func TestParseGateMode(t *testing.T) {
	for in, want := range map[string]GateMode{"": GateOff, "off": GateOff, "warn": GateWarn, "block_on_critical": GateBlockOnCritical} {
		if got, err := ParseGateMode(in); err != nil || got != want {
			t.Errorf("ParseGateMode(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := ParseGateMode("block"); !errors.Is(err, ErrInvalid) {
		t.Errorf("expected ErrInvalid, got %v", err)
	}
}
