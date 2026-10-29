// GET/POST/DELETE /api/v1/databases/{name}/pooler (internal/api/database_pooler.go).
// URLs carry a password placeholder, never a real credential.
export interface DatabasePoolerResource {
  enabled: boolean
  supported: boolean
  image_configured: boolean
  container?: string
  pooled_url?: string
  direct_url?: string
  status?: 'running' | 'failed' | 'unknown'
  reason?: string
  message?: string
}
