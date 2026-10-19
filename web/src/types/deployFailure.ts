/** Mirrors internal/failure.Fix: a typed suggestion matched from the build log. */
export interface DeployFailureFix {
  code: string
  kind: 'rerun' | 'raise_memory' | 'free_disk' | 'set_secret' | 'edit_source'
  target?: string
  hint: string
  match: string
}

/** Mirrors internal/failure.Failure on the wire. */
export interface DeployFailure {
  code: string
  cause: string
  failing_step?: string
  log_excerpt?: string
  suggested_fix: string
  docs_url: string
  retryable: boolean
  also_matched?: string[]
  fixes?: DeployFailureFix[]
  deploy_id: string
  app: string
  at: string
}
