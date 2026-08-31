// Wire type for GET /api/v1/apps/{name}/diagnose
// (internal/api/diagnose.go's handleDiagnoseApp, diagnosisResource): a
// read-only, deterministic explanation of why an app's most recent
// deploy attempt failed or why it's crashlooping, synthesized from
// internal/diagnose over signals the platform already collects. Never
// backed by an external model call and never changes anything.
export type DiagnosisConfidence = 'high' | 'medium' | 'none'

export interface DiagnosisSignal {
  source: string
  excerpt: string
}

export interface Diagnosis {
  explanation: string
  suggestion: string
  confidence: DiagnosisConfidence
  matched_signals: DiagnosisSignal[]
  deploy_attempt_id?: string
}
