export type PreflightStatus = 'pass' | 'warn' | 'fail' | 'unknown'

// Matches internal/nodepreflight.Check.
export interface PreflightCheck {
  id: string
  status: PreflightStatus
  detail: string
  fix?: string
  fix_command?: string
}

export interface PreflightSummary {
  pass: number
  warn: number
  fail: number
  unknown: number
  ready: boolean
}

export type OnboardingStage =
  'waiting_for_agent' | 'connected' | 'ready' | 'needs_attention'

export interface OnboardingDomain {
  provider?: string
  region?: string
  zone?: string
  instance_id?: string
  source?: string
  metadata_disabled?: boolean
}

// Matches internal/api's nodeOnboardingResponse.
export interface NodeOnboarding {
  node_id: string
  stage: OnboardingStage
  reported: boolean
  stale: boolean
  reported_at?: string
  detected?: OnboardingDomain
  domain: OnboardingDomain
  checks: PreflightCheck[]
  summary: PreflightSummary
}

// Matches internal/placement.DomainGroup. Capacity sums only what nodes
// reported, unverified_nodes counts the ones that reported nothing.
export interface DomainGroup {
  name: string
  region?: string
  providers?: string[]
  node_ids: string[]
  nodes: number
  online_nodes: number
  schedulable_nodes: number
  workloads: number
  cpu_milli: number
  memory_bytes: number
  disk_free_bytes: number
  unverified_nodes: number
}

export interface DomainRisk {
  kind: 'replicas_one_zone' | 'replicas_one_provider'
  level: 'warn' | 'info'
  name: string
  node_id: string
  replicas: number
  zone?: string
  region?: string
  provider?: string
  alternatives: string[]
  message: string
}

export interface NodeDomains {
  providers: DomainGroup[]
  regions: DomainGroup[]
  zones: DomainGroup[]
  unlabeled_nodes: number
  total_nodes: number
  risks: DomainRisk[]
}
