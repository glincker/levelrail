// Wire types for GET /api/v1/environments/{id}/clone/preview and
// POST /api/v1/environments/{id}/clone (internal/api/environment_clone.go):
// cloning a whole environment's app set plus config into a new
// environment, distinct from PromoteAppDialog's own single-app image
// move (types/promote.ts).

import type { EnvironmentResource } from './environment'

export interface EnvironmentCloneAppPreview {
  source_app: string
  suggested_new_name: string
  image: string
  env_var_count: number
  secret_env_keys: string[]
  current_domains: string[]
  volume_count: number
  bind_mount_count: number
  has_health_check: boolean
  has_database_attachment: boolean
  has_host_port_pin: boolean
  scheduled_task_count: number
}

export interface EnvironmentClonePreviewResource {
  source_environment: EnvironmentResource
  new_environment_name: string
  apps: EnvironmentCloneAppPreview[]
  environment_env_var_keys: string[]
  environment_secret_env_keys: string[]
  uncloned_fields: string[]
  note: string
}

// EnvironmentCloneAppInput overrides one source app's own new name
// and/or domains inside a clone request; a source app with no entry
// here uses the server's own auto-suggested name and gets no domains.
export interface EnvironmentCloneAppInput {
  source_app: string
  new_name?: string
  domains?: string[]
}

export interface EnvironmentCloneAppResult {
  source_app: string
  new_app: string
  image: string
}

export interface EnvironmentCloneResultResource {
  environment: EnvironmentResource
  apps: EnvironmentCloneAppResult[]
  copied_secret_values: boolean
  note: string
}
