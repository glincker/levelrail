export interface LogUsageApp {
  app: string
  bytes: number
  lines: number
  rate_bytes_per_min: number
  budget_bytes: number
  budget_custom: boolean
  percent: number | null
  forecast_days: number | null
  trimmed_lines: number
  trimmed_bytes: number
}

export interface LogUsage {
  default_budget_bytes: number
  apps: LogUsageApp[]
}

export interface LogBudget {
  max_mb: number
  default_mb: number
  effective_max_bytes: number
}
