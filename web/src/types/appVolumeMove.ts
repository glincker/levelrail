// Wire types for "move this app to another node, taking its volumes with
// it" attempts, matching internal/api/apps_move_with_volumes.go's
// appVolumeMoveResource. POST /api/v1/apps/{name}/move-with-volumes always
// returns this shape: Status "succeeded" means it already finished (an app
// with no volumes, or already on the target node, both handled
// synchronously), Status "running" means the caller should poll
// GET .../moves/{id} until it isn't anymore.

export type AppVolumeMoveStatus = 'running' | 'succeeded' | 'failed'

export interface AppVolumeMoveStep {
  name: string
  status: AppVolumeMoveStatus
  error?: string
  started_at: string
  finished_at?: string
}

export interface AppVolumeMove {
  id: string
  service_name: string
  from_node_id: string
  to_node_id: string
  status: AppVolumeMoveStatus
  error?: string
  steps: AppVolumeMoveStep[]
  started_at: string
  finished_at?: string
}

export interface MoveAppWithVolumesRequest {
  node_id: string
}
