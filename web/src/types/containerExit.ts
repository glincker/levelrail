export interface ContainerExitLine {
  timestamp: string
  stream: string
  message: string
}

/** A release container that stopped before it could serve, as captured by
 *  the reconciler (internal/api/app_run_state.go deployExitResource). */
export interface ContainerExit {
  container: string
  exit_code: number
  oom_killed?: boolean
  reason: string
  message: string
  started_at?: string
  finished_at?: string
  captured_at: string
  deploy_id?: string
  lines: ContainerExitLine[]
}

/** The live log stream's `event: status` payload (appRunStatus). */
export interface AppRunStatus {
  running: boolean
  container?: string
  exit_code?: number
  oom_killed?: boolean
  finished_at?: string
  reason?: string
  message?: string
}
