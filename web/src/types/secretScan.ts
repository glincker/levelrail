// Shapes of the secret scan routes (internal/api/secretscan.go). A finding
// never carries the matched value, only a rule id, a location and a fingerprint.

export type SecretScanPolicy = 'off' | 'warn' | 'block'

export type SecretScanAction = 'skipped' | 'allow' | 'warn' | 'block'

export interface SecretFinding {
  rule_id: string
  source: 'file' | 'env'
  path?: string
  line?: number
  var?: string
  fingerprint: string
  allowed?: boolean
}

export interface SecretScanAllowEntry {
  fingerprint: string
  reason: string
  actor: string
  created_at: string
}

export interface SecretScanLimits {
  max_files: number
  max_file_bytes: number
  max_total_bytes: number
  max_findings: number
  timeout_seconds: number
}

export interface SecretScanSettings {
  app: string
  policy: SecretScanPolicy
  allow_list: SecretScanAllowEntry[]
  limits: SecretScanLimits
}

export interface SecretScanRun {
  deployment_id: string
  app: string
  policy: SecretScanPolicy
  action: SecretScanAction
  reason?: string
  status: 'complete' | 'partial' | 'error'
  error?: string
  files_scanned: number
  truncated: boolean
  findings: SecretFinding[]
  scanned_at: string
}
