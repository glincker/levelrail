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
  deploy_id: string
  app: string
  at: string
}
