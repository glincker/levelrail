// Wire shapes of /api/v1/costs (internal/usagecost/explore.go, internal/costbudget, internal/api/cost_*.go).

export type CostGroupBy = 'app' | 'project' | 'tag' | 'node' | 'kind'
export type CostGranularity = 'daily' | 'monthly'
export type CostPeriod = 'mtd' | '30d' | 'last_month'
export type CostConfidence = 'low' | 'medium' | 'high'
export type CostBudgetScope = 'project' | 'tag' | 'app'
export type CostBudgetAction = 'none' | 'throttle' | 'pause'
export type SpendGuardMode = 'alert' | 'block_deploys'
export type CostBudgetLevel = '' | 'warn' | 'over' | 'forecast'

export interface CostSeries {
  key: string
  total: number
  previous: number
  delta: number
  delta_pct?: number
  values: number[]
}

export interface CostForecastBand {
  spent: number
  mid: number
  low: number
  high: number
  confidence: CostConfidence
  samples: number
}

export interface CostExploration {
  group_by: CostGroupBy
  granularity: CostGranularity
  currency: string
  period: { from: string; to: string }
  buckets: string[]
  trend: number[]
  total: number
  previous: number
  delta_pct?: number
  series: CostSeries[]
  movers: CostSeries[]
  labels: Record<string, string>
  forecast?: CostForecastBand
  configured: boolean
  telemetry_available: boolean
  generated_at: string
}

export interface CostExploreParams {
  groupBy: CostGroupBy
  granularity: CostGranularity
  period: CostPeriod
  months: number
}

export interface CostBudget {
  id: string
  name: string
  scope: CostBudgetScope
  scope_value: string
  monthly: number
  thresholds: number[]
  action: CostBudgetAction
  throttle_rps: number
  guard_mode: SpendGuardMode
}

export interface CostBudgetStatus extends CostBudget {
  currency: string
  period: string
  spent: number
  pct: number
  forecast: CostForecastBand
  forecast_pct: number
  highest: number
  enforced: string
  apps: string[]
  level: CostBudgetLevel
  suggested_monthly: number
  resets_at: string
}

export interface CostBudgets {
  budgets: CostBudgetStatus[]
  currency: string
}

export type CostBudgetInput = Partial<Omit<CostBudget, 'id'>>
