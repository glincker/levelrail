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

export interface DetectionReason {
  signal: string
  detail: string
}

export interface DetectionEnvVar {
  name: string
  source: string
  required: boolean
  secret_guess: boolean
}

export interface Detection {
  framework: string
  language: string
  build_type: string
  confidence: 'high' | 'medium' | 'low'
  reasons: DetectionReason[]
  port: { value: number; source: string }
  start_command?: string
  build_command?: string
  output_dir?: string
  health_path: string
  runtime_version: { value: string; source: string }
  env_vars: DetectionEnvVar[]
  warnings: AnalyzeWarning[]
}

export interface WorkspacePackage {
  name: string
  dir: string
  tool: string
  dockerfile?: string
  depends_on: string[]
  watch_paths: string[]
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
  detection?: Detection
  packages?: WorkspacePackage[]
  files_read: number
  tree_truncated: boolean
}

type NullableList<T> = T[] | null | undefined

export type RawImportAnalysis = Omit<
  ImportAnalysis,
  'env' | 'databases' | 'warnings' | 'build'
> & {
  env: NullableList<AnalyzeEnvKey>
  databases: NullableList<AnalyzeDatabase>
  warnings: NullableList<AnalyzeWarning>
  build: Omit<AnalyzeBuild, 'alternatives'> & {
    alternatives: NullableList<AnalyzeBuildSuggestion>
  }
}

/** Older servers send null for empty lists; the UI iterates them directly. */
export function normalizeAnalysis(raw: RawImportAnalysis): ImportAnalysis {
  return {
    ...raw,
    env: raw.env ?? [],
    databases: raw.databases ?? [],
    warnings: raw.warnings ?? [],
    packages: raw.packages ?? [],
    build: { ...raw.build, alternatives: raw.build.alternatives ?? [] },
  }
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
  return normalizeAnalysis((await res.json()) as RawImportAnalysis)
}
