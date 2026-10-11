import { ApiError, readErrorMessage } from '../lib/apiError'

// POST /api/v1/imports/analyze (internal/api/imports_analyze.go): a
// read-only, provider-API analysis of one repository. Nothing is created.

export interface AnalyzeText {
  value: string
  evidence: string
}

export interface AnalyzeBuildSuggestion {
  build_type: string
  dockerfile_path?: string
  base_directory?: string
  reason: string
  reason_code: string
  recommended: boolean
}

export interface AnalyzeBuild {
  type: string
  dockerfile_path?: string
  base_directory?: string
  evidence: string
  alternatives: AnalyzeBuildSuggestion[]
}

export interface AnalyzeEnvKey {
  key: string
  required: boolean
  secret: boolean
  source: string
  provided_by?: string
  has_default: boolean
}

export interface AnalyzeDatabase {
  engine: string
  version: string
  suggested_name: string
  env_var: string
  field: string
  evidence: string
}

export interface AnalyzeWarning {
  code: string
  message: string
}

export interface ImportAnalysis {
  repo_url: string
  branch: string
  monorepo: boolean
  app_directory?: string
  name: AnalyzeText
  stack: { runtime: string; framework?: string; evidence: string }
  build: AnalyzeBuild
  port: { value: number; evidence: string }
  health: AnalyzeText
  memory: { bytes: number; evidence: string }
  domain: AnalyzeText
  env: AnalyzeEnvKey[]
  databases: AnalyzeDatabase[]
  warnings: AnalyzeWarning[]
  files_read: number
  tree_truncated: boolean
}

export interface ImportAnalyzeRequest {
  repo_url: string
  ref?: string
  base_directory?: string
  dockerfile_path?: string
  build_type?: string
  name?: string
  /** One-shot deploy token for a private repository pasted by URL. */
  token?: string
}

export async function analyzeImport(
  req: ImportAnalyzeRequest,
): Promise<ImportAnalysis> {
  const res = await fetch('/api/v1/imports/analyze', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `analyze repository failed: ${res.status}`),
    )
  }
  return (await res.json()) as ImportAnalysis
}
