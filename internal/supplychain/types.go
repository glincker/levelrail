// Package supplychain records the SBOM of each build, optionally scans it for
// known vulnerabilities in a short-lived container, and decides whether a
// release may go live.
package supplychain

import (
	"errors"
	"fmt"
	"time"
)

// SBOM document formats.
const (
	FormatSPDX      = "spdx"
	FormatCycloneDX = "cyclonedx"
)

// Package is one component found in an SBOM.
type Package struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Type    string `json:"type,omitempty"`
	License string `json:"license,omitempty"`
}

// LicenseCount is how many packages carry a license.
type LicenseCount struct {
	License string `json:"license"`
	Count   int    `json:"count"`
}

// TypeCount is how many packages are of a type.
type TypeCount struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// SBOMSummary is the compact view stored in the database.
type SBOMSummary struct {
	Format       string         `json:"format"`
	PackageCount int            `json:"package_count"`
	Types        []TypeCount    `json:"types,omitempty"`
	Licenses     []LicenseCount `json:"licenses,omitempty"`
	Unlicensed   int            `json:"unlicensed"`
	TopPackages  []Package      `json:"top_packages,omitempty"`
}

// Severity levels, most severe first.
const (
	SeverityCritical = "critical"
	SeverityHigh     = "high"
	SeverityMedium   = "medium"
	SeverityLow      = "low"
	SeverityUnknown  = "unknown"
)

// Counts is the number of findings per severity.
type Counts struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Unknown  int `json:"unknown"`
}

// Total is the number of findings across every severity.
func (c Counts) Total() int { return c.Critical + c.High + c.Medium + c.Low + c.Unknown }

func (c *Counts) add(sev string) {
	switch sev {
	case SeverityCritical:
		c.Critical++
	case SeverityHigh:
		c.High++
	case SeverityMedium:
		c.Medium++
	case SeverityLow:
		c.Low++
	default:
		c.Unknown++
	}
}

// Vuln is one finding.
type Vuln struct {
	ID           string `json:"id"`
	Package      string `json:"package"`
	Version      string `json:"version,omitempty"`
	FixedVersion string `json:"fixed_version,omitempty"`
	Severity     string `json:"severity"`
	Title        string `json:"title,omitempty"`
}

// ScanSummary is what a scanner reported for one SBOM.
type ScanSummary struct {
	Scanner string `json:"scanner"`
	Counts  Counts `json:"counts"`
	// Fixable is the number of findings that have a fixed version.
	Fixable int `json:"fixable"`
	// Top lists the most severe, fixable-first findings.
	Top []Vuln `json:"top,omitempty"`
}

// Scan statuses stored on a record.
const (
	ScanNone        = ""
	ScanOK          = "ok"
	ScanFailed      = "failed"
	ScanUnavailable = "unavailable"
)

// Record is the per-deploy-attempt supply chain metadata.
type Record struct {
	AttemptID     string
	App           string
	Summary       SBOMSummary
	SBOMBytes     int64
	HasProvenance bool
	GeneratedAt   time.Time

	ScanStatus string
	Scanner    string
	ScanError  string
	ScannedAt  time.Time
	Scan       *ScanSummary

	GateAction string
	GateReason string
}

// GateMode is what a scan result may do to a release.
type GateMode string

// Gate modes, least strict first.
const (
	GateOff             GateMode = "off"
	GateWarn            GateMode = "warn"
	GateBlockOnCritical GateMode = "block_on_critical"
)

// Sentinel errors.
var (
	ErrInvalid  = errors.New("supplychain: invalid setting")
	ErrNotFound = errors.New("supplychain: not found")
	ErrDisabled = errors.New("supplychain: scanning is disabled on this server")
	ErrNoSBOM   = errors.New("supplychain: deployment has no SBOM")
)

// ParseGateMode validates a gate mode string; empty means off.
func ParseGateMode(s string) (GateMode, error) {
	switch m := GateMode(s); m {
	case "":
		return GateOff, nil
	case GateOff, GateWarn, GateBlockOnCritical:
		return m, nil
	}
	return "", fmt.Errorf("%w: scan_gate must be off, warn or block_on_critical", ErrInvalid)
}

// Settings are the per-app scan settings.
type Settings struct {
	App     string
	Enabled bool
	Gate    GateMode
	// OverrideReason and OverrideArmedAt describe a one-shot gate override.
	OverrideReason  string
	OverrideArmedAt time.Time
}
