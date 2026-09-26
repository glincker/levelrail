package supplychain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Scanner names.
const (
	ScannerTrivy = "trivy"
	ScannerGrype = "grype"
)

const topVulnLimit = 10

type trivyReport struct {
	Results []struct {
		Vulnerabilities []struct {
			ID           string `json:"VulnerabilityID"`
			Package      string `json:"PkgName"`
			Version      string `json:"InstalledVersion"`
			FixedVersion string `json:"FixedVersion"`
			Severity     string `json:"Severity"`
			Title        string `json:"Title"`
		} `json:"Vulnerabilities"`
	} `json:"Results"`
}

type grypeReport struct {
	Matches []struct {
		Vulnerability struct {
			ID          string `json:"id"`
			Severity    string `json:"severity"`
			Description string `json:"description"`
			Fix         struct {
				Versions []string `json:"versions"`
			} `json:"fix"`
		} `json:"vulnerability"`
		Artifact struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"artifact"`
	} `json:"matches"`
}

func normalizeSeverity(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case SeverityCritical:
		return SeverityCritical
	case SeverityHigh:
		return SeverityHigh
	case SeverityMedium:
		return SeverityMedium
	case SeverityLow, "negligible":
		return SeverityLow
	}
	return SeverityUnknown
}

// ParseTrivy reads `trivy sbom --format json` output.
func ParseTrivy(data []byte) (ScanSummary, error) {
	var rep trivyReport
	if err := json.Unmarshal(data, &rep); err != nil {
		return ScanSummary{}, fmt.Errorf("supplychain: parse trivy output: %w", err)
	}
	var vulns []Vuln
	for _, r := range rep.Results {
		for _, v := range r.Vulnerabilities {
			vulns = append(vulns, Vuln{ID: v.ID, Package: v.Package, Version: v.Version, FixedVersion: v.FixedVersion, Severity: normalizeSeverity(v.Severity), Title: v.Title})
		}
	}
	return summarizeVulns(ScannerTrivy, vulns), nil
}

// ParseGrype reads `grype -o json` output.
func ParseGrype(data []byte) (ScanSummary, error) {
	var rep grypeReport
	if err := json.Unmarshal(data, &rep); err != nil {
		return ScanSummary{}, fmt.Errorf("supplychain: parse grype output: %w", err)
	}
	var vulns []Vuln
	for _, m := range rep.Matches {
		fixed := ""
		if len(m.Vulnerability.Fix.Versions) > 0 {
			fixed = strings.Join(m.Vulnerability.Fix.Versions, ", ")
		}
		vulns = append(vulns, Vuln{ID: m.Vulnerability.ID, Package: m.Artifact.Name, Version: m.Artifact.Version, FixedVersion: fixed, Severity: normalizeSeverity(m.Vulnerability.Severity), Title: firstLine(m.Vulnerability.Description)})
	}
	return summarizeVulns(ScannerGrype, vulns), nil
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	if len(line) > 200 {
		line = line[:200]
	}
	return line
}

func severityRank(s string) int {
	switch s {
	case SeverityCritical:
		return 0
	case SeverityHigh:
		return 1
	case SeverityMedium:
		return 2
	case SeverityLow:
		return 3
	}
	return 4
}

func summarizeVulns(scanner string, vulns []Vuln) ScanSummary {
	seen := map[string]bool{}
	uniq := vulns[:0:0]
	for _, v := range vulns {
		key := v.ID + "|" + v.Package + "|" + v.Version
		if seen[key] {
			continue
		}
		seen[key] = true
		uniq = append(uniq, v)
	}
	sum := ScanSummary{Scanner: scanner}
	for _, v := range uniq {
		sum.Counts.add(v.Severity)
		if v.FixedVersion != "" {
			sum.Fixable++
		}
	}
	sort.SliceStable(uniq, func(i, j int) bool {
		if ri, rj := severityRank(uniq[i].Severity), severityRank(uniq[j].Severity); ri != rj {
			return ri < rj
		}
		if fi, fj := uniq[i].FixedVersion != "", uniq[j].FixedVersion != ""; fi != fj {
			return fi
		}
		return uniq[i].ID < uniq[j].ID
	})
	if len(uniq) > topVulnLimit {
		uniq = uniq[:topVulnLimit]
	}
	sum.Top = uniq
	return sum
}
