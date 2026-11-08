// Wire types for GET /api/v1/apps/{name}/containers
// (internal/api/app_containers.go): one entry per replica container.
export interface AppContainerHealth {
  status: string
  failing_streak: number
  last_output: string
}

export interface AppContainerPort {
  container: number
  host: number
  proto: string
  bind: string
}

export interface AppContainerNetwork {
  name: string
  ip: string
  aliases: string[]
}

export interface AppContainerMount {
  type: string
  source: string
  target: string
  ro: boolean
}

export interface AppContainerStats {
  cpu_percent: number
  memory_bytes: number
  memory_limit_bytes: number
}

export interface AppContainer {
  id: string
  name: string
  image: string
  image_digest: string
  state: string
  restart_count?: number
  started_at?: string
  node: string
  health?: AppContainerHealth
  ports: AppContainerPort[]
  networks: AppContainerNetwork[]
  mounts: AppContainerMount[]
  limits: { cpu: number; memory_bytes: number }
  stats?: AppContainerStats
  /** Fields the replica's node cannot report; never read an empty value as real. */
  unavailable: string[]
}
