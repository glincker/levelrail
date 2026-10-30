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

export interface AnalysisReportBody {
  status: AnalysisStatus
  summary: string
  timeline: AnalysisEvent[] | null
  causes: AnalysisCause[] | null
  recommendations: AnalysisRecommendation[] | null
}

export interface AnalysisReportDetail extends AnalysisReportSummary {
  schema_version: number
  report: AnalysisReportBody
}
