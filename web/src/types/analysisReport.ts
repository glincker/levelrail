export type AnalysisStatus = 'complete' | 'insufficient_data'

export interface AnalysisReportSummary {
  id: string
  app: string
  trigger_kind: string
  trigger_id: string
  status: AnalysisStatus
  summary: string
  created_at: string
}

export interface AnalysisEvent {
  at: string
  kind: string
  ref: string
  detail?: string
}

export interface AnalysisCause {
  rank: number
  id: string
  title: string
  score: number
  confidence: string
  evidence: string[]
}

export interface AnalysisRecommendation {
  id: string
  text: string
  monthly_saving_usd?: number
  evidence: string[]
}

export interface AnalysisMetricStat {
  evidence: string
  metric: string
  samples: number
  min: number
  p50: number
  p95: number
  max: number
  mean: number
  baseline_mean: number
  trend_pct: number
  trend: string
  anomaly_count: number
  max_z: number
  first_anomaly?: string
}

export interface AnalysisLogGroup {
  evidence: string
  level: string
  fingerprint: string
  sample: string
  count: number
  first_seen: string
  last_seen: string
}

export interface AnalysisSizing {
  dimension: string
  action: string
  confidence: string
  data_sufficient: boolean
  current_limit: number
  p95_usage: number
  suggested_limit: number
  suggested_text?: string
}

export interface AnalysisDigest {
  metrics: AnalysisMetricStat[] | null
  logs: AnalysisLogGroup[] | null
  log_lines: number
  restarts: number
  sizing?: AnalysisSizing[] | null
  monthly_saving_usd?: number
}

export interface AnalysisReportBody {
  status: AnalysisStatus
  summary: string
  timeline: AnalysisEvent[] | null
  causes: AnalysisCause[] | null
  recommendations: AnalysisRecommendation[] | null
  digest?: AnalysisDigest | null
}

export interface AnalysisReportDetail extends AnalysisReportSummary {
  schema_version: number
  report: AnalysisReportBody
}
