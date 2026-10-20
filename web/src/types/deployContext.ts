export type ConfigSource =
  'app' | 'preview-env' | 'context' | 'branch-context' | 'branch-env'

export interface ContextConfig {
  context: string
  env: Record<string, string>
  build_command: string
  publish_dir: string
  build_args: Record<string, string>
  updated_at?: string
}

export interface EffectiveEnvEntry {
  key: string
  value: string
  secret: boolean
  source: ConfigSource
}

export interface EffectiveSetting {
  value: string
  source: ConfigSource
}

export interface EffectiveConfig {
  context: string
  env: EffectiveEnvEntry[] | null
  build_command: EffectiveSetting
  publish_dir: EffectiveSetting
  build_args?: Record<string, EffectiveSetting>
}
