// Wire type shared by GET /api/v1/apps/{name}/resource-recommendation and
// GET /api/v1/databases/{name}/resource-recommendation
// (internal/api's resourceRecommendationResource): a read-only,
// deterministic memory/CPU right-sizing suggestion derived from the
// resource's own historical usage (internal/rightsizing), never from an
// external model, and never applied automatically.
export type RecommendationConfidence = 'high' | 'medium' | 'low'

export type RecommendationAction = 'raise' | 'lower' | 'keep' | ''

export interface DimensionRecommendation {
  dimension: 'memory' | 'cpu'
  sample_count: number
  data_sufficient: boolean
  confidence: RecommendationConfidence
  current_limit: number
  p95_usage: number
  p99_usage: number
  suggested_limit: number
  action: RecommendationAction
  reason: string
}

export interface ResourceRecommendation {
  service_name: string
  lookback_window: string
  memory: DimensionRecommendation
  cpu: DimensionRecommendation
  oom_detected_at?: string
  oom_excerpt?: string
}
