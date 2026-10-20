// Wire types for the app-integration resource, matching
// internal/api/app_integrations.go's integrationCatalogEntry and
// appIntegrationResource exactly: same field names, same snake_case
// JSON tags.

// Drives which input affordance a catalog field's form control shows;
// purely descriptive, matching internal/integrations.EnvVar.Type.
export type IntegrationFieldType =
  'api_key' | 'dsn' | 'token' | 'project_id' | 'site' | 'host'

export interface IntegrationCatalogEnvVar {
  name: string
  type: IntegrationFieldType
  required: boolean
  default?: string
  placeholder?: string
}

// GET /api/v1/integrations response entry: one internal/integrations
// catalog tool (Sentry, PostHog, etc), static and global.
export interface IntegrationCatalogEntry {
  key: string
  name: string
  description: string
  docs_url: string
  env_vars: IntegrationCatalogEnvVar[]
  frameworks?: string[]
}

// GET/POST /api/v1/apps/{name}/integrations response shape. Field
// values (API keys, DSNs) are never included, matching the "names
// only, never a value" convention every secret-backed resource in this
// app already follows.
export interface AppIntegration {
  id: string
  app_name?: string
  integration_key: string
  name: string
  created_at?: string
  updated_at?: string
}

// POST /api/v1/apps/{name}/integrations request body.
export interface AttachAppIntegrationRequest {
  integration_key: string
  fields: Record<string, string>
}
