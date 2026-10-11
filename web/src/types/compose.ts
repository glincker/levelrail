// Wire shapes for compose stacks (internal/api/compose_stack.go,
// compose_revisions.go, compose_preview.go) and template manifests
// (internal/catalog/manifest.go).

export type ComposePhase =
  'running' | 'progressing' | 'degraded' | 'failed' | 'stopped'

export interface ComposeStackService {
  key: string
  name: string
  image: string
  port?: number
  domains?: string[]
  depends_on?: string[]
  depends_on_healthy?: string[]
  volumes?: string[]
  replicas: number
  suspended?: boolean
  ready: 'True' | 'False' | 'Unknown'
  reason?: string
  message?: string
  since?: string
}

export interface ComposeStack {
  app_id: string
  phase: ComposePhase
  revision?: number
  deployed_at?: string
  source?: string
  template_id?: string
  profiles?: string[]
  services: ComposeStackService[]
}

export interface ComposeRevision {
  revision: number
  source: string
  template_id?: string
  profiles?: string[]
  created_by?: string
  created_at: string
  compose?: string
}

export interface ComposePreviewEnv {
  key: string
  value?: string
  secret?: boolean
  generated?: boolean
  required?: boolean
}

export interface ComposePreviewService {
  key: string
  image?: string
  build?: boolean
  port?: number
  domain?: string
  volumes?: string[]
  bind_mounts?: string[]
  env?: ComposePreviewEnv[]
  depends_on?: string[]
  depends_on_healthy?: string[]
  profiles?: string[]
  healthcheck?: string
}

export interface ComposePreview {
  valid: boolean
  errors?: string[]
  services: ComposePreviewService[]
  profiles?: string[]
  skipped_services?: string[]
  notices?: { level: 'note' | 'warning'; message: string }[]
  warnings?: string[]
  needs_root?: boolean
}

export interface ComposeDownResult {
  app_id: string
  removed: string[]
  teardown_pending?: string[]
  volumes_kept?: string[]
  volumes_removed?: string[]
  volume_errors?: string[]
}

export interface TemplateEnvPrompt {
  key: string
  label?: string
  description?: string
  default?: string
  generate?: 'password' | 'user' | 'hex' | 'base64'
  length?: number
  required?: boolean
  secret?: boolean
}

export interface TemplateDomainPrompt {
  service: string
  port: number
  description?: string
}

export interface TemplateSource {
  name: string
  url: string
  license: string
  path: string
}

// One service of an installable template as the catalog describes it.
export interface TemplateStackService {
  name: string
  image: string
  ports?: number[]
  volumes?: string[]
  depends_on?: string[]
}

export interface TemplateManifest {
  id: string
  name: string
  description: string
  icon?: string
  category: string
  documentation_url: string
  env?: TemplateEnvPrompt[]
  domains?: TemplateDomainPrompt[]
  healthcheck?: { service: string; port: number; path: string }
  source?: TemplateSource
  images?: { service: string; image: string }[]
}

export interface TemplateInstallInput {
  name?: string
  env?: Record<string, string>
  domains?: Record<string, string>
}
