// Wire types for the per-app stream resource, matching
// internal/api/app_streams.go's appStreamResource.

export type AppStreamProtocol = 'tcp'

export interface AppStream {
  id: string
  app: string
  container_port: number
  host_port: number
  protocol: AppStreamProtocol
  created_at: string
}

export interface CreateAppStreamRequest {
  container_port: number
  host_port: number
  protocol?: AppStreamProtocol
}
