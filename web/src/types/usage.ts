// Wire shapes of /api/v1/usage (internal/usagecost and internal/api/usage_cost*.go).

export type UsagePeriod = 'mtd' | '30d' | 'last_month'
export type UsageMode = 'unit' | 'node'
export type UsageBasis = 'avg' | 'p50'
export type UsageDimension =
  'project' | 'environment' | 'team' | 'resource' | 'tag'

export interface UsageGroup {
  key: string
  kind?: string
  name?: string
  total: number
  compute: number
  storage: number
  egress: number
  idle_share: number
  previous: number
  delta: number
  delta_pct?: number
  project_id?: string
  team_id?: string
  environment_id?: string
  daily: number[]
}

export interface UsageTotals {
  total: number
  allocated: number
  unallocated: number
  previous: number
  delta_pct?: number
}

export interface UsageReport {
  period: { from: string; to: string }
  previous: { from: string; to: string }
  days: string[]
  totals: UsageTotals
  resources: UsageGroup[]
  projects: UsageGroup[]
  environments: UsageGroup[]
  teams: UsageGroup[]
  movers: UsageGroup[]
  tags?: UsageGroup[]
  forecast?: number
  currency: string
  mode: UsageMode
  basis: UsageBasis
  labels: Record<string, string>
  unpriced_nodes: string[]
  includes_unallocated: boolean
  configured: boolean
  telemetry_available: boolean
  generated_at: string
}

export interface UsageSettingsValues {
  mode: UsageMode
  currency: string
  cpu_core_hour: number
  mem_gib_hour: number
  storage_gib_month: number
  egress_gib: number
  idle_spread_pct: number
  basis: UsageBasis
  cpu_weight_pct: number
}

export interface UsageNodeInfo {
  node_id: string
  name: string
  monthly_price: number
  vcpus: number
  memory_gib: number
  detected_vcpus: number
  detected_memory_gib: number
  priced: boolean
}

export interface UsagePreset {
  key: string
  label: string
  monthly_price: number
  vcpus: number
  memory_gib: number
  cpu_core_hour: number
  mem_gib_hour: number
}

export interface UsageSettings {
  settings: UsageSettingsValues
  nodes: UsageNodeInfo[]
  presets: UsagePreset[]
  note: string
}

export interface UsageSettingsUpdate {
  settings?: UsageSettingsValues
  nodes?: Array<{
    node_id: string
    monthly_price: number
    vcpus: number
    memory_gib: number
  }>
  reprice_month?: boolean
}

export type BudgetLevel = '' | 'warn' | 'over' | 'forecast'

export interface UsageBudget {
  id: string
  scope: 'project' | 'team'
  scope_id: string
  monthly: number
  warn_pct: number
  name: string
  currency: string
  spent: number
  forecast: number
  spent_pct: number
  forecast_pct: number
  level: BudgetLevel
}

export interface UsageBudgets {
  budgets: UsageBudget[]
  currency: string
}
