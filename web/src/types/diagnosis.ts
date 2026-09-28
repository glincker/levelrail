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
  causes?: DiagnosisCause[]
  fixable?: boolean
}

export type DiagnosisFixKind = 'patch' | 'input' | 'manual'

// One field edit a fix proposes, on the app resource's own JSON paths:
// "port", "health.readiness.path", "resources.memory_bytes", "env.NAME".
export interface DiagnosisChange {
  field: string
  from: string
  to: string
  needs_input?: boolean
}

export interface DiagnosisFix {
  n: number
  label: string
  kind: DiagnosisFixKind
  changes: DiagnosisChange[]
  hint?: string
  redeploy?: boolean
}

export interface DiagnosisCause {
  code: string
  title: string
  explanation: string
  confidence: DiagnosisConfidence
  evidence: DiagnosisSignal[]
  fixes: DiagnosisFix[]
}
