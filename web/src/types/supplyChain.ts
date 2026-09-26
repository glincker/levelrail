// Wire types for the supply chain routes (internal/api/supplychain.go):
// GET/PUT /api/v1/apps/{name}/supply-chain, POST .../supply-chain/override,
// GET .../deployments/{id}/sbom, GET .../deployments/{id}/vulnerabilities and
// POST .../deployments/{id}/scan.

export type ScanGate = 'off' | 'warn' | 'block_on_critical'

export type VulnSeverity = 'critical' | 'high' | 'medium' | 'low' | 'unknown'

export interface VulnCounts {
  critical: number
  high: number
  medium: number
  low: number
  unknown: number
}

export interface SbomPackage {
  name: string
  version?: string
  type?: string
  license?: string
}

export interface SbomSummary {
  deployment_id: string
  format: 'spdx' | 'cyclonedx'
  package_count: number
  types: { type: string; count: number }[]
  licenses: { license: string; count: number }[]
  /** Packages that declare no license. */
  unlicensed: number
  top_packages: SbomPackage[]
  provenance: boolean
  /** False once retention removed the document; the summary stays. */
  available: boolean
  bytes: number
  generated_at: string
  download_url?: string
}

export interface Vulnerability {
  id: string
  package: string
  version?: string
  fixed_version?: string
  severity: VulnSeverity
  title?: string
}

export type ScanStatus = '' | 'ok' | 'failed' | 'unavailable'

export type GateAction = 'allow' | 'warn' | 'block' | 'override'

export interface VulnReport {
  deployment_id: string
  scan: {
    status: ScanStatus
    scanner?: string
    scanned_at?: string
    error?: string
    counts?: VulnCounts
    fixable: number
    top: Vulnerability[]
  }
  gate?: { action: GateAction; reason?: string }
}

export interface SupplyChainSettings {
  app: string
  scan_enabled: boolean
  scan_gate: ScanGate
  /** False when the server runs with APP_SCAN_ENABLED=false. */
  server_enabled: boolean
  /** False when builds produce no SBOM (APP_BUILD_ATTEST is not true). */
  build_attest: boolean
  scanner: string
  scanner_image: string
  override_armed: boolean
  override_reason?: string
  override_expires_at?: string
}

export interface SupplyChainSettingsInput {
  scan_enabled?: boolean
  scan_gate?: ScanGate
}
