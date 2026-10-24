// Matches the runtime settings API (internal/runtimesettings, /api/v1/settings/runtime).

export type RuntimeSettingCategory =
  | 'security'
  | 'networking'
  | 'deploys'
  | 'observability'
  | 'experimental'
  | 'retention'

export type RuntimeSettingKind = 'bool' | 'duration' | 'string_list' | 'int'
export type RuntimeSettingSource = 'saved' | 'env' | 'default'
export type RuntimeSettingApply = 'live' | 'restart'
export type RuntimeSettingRisk = 'normal' | 'weakens_security'

export type RuntimeSettingValue = boolean | string | number | string[]

export interface RuntimeSetting {
  key: string
  category: RuntimeSettingCategory
  kind: RuntimeSettingKind
  title: string
  description: string
  default: RuntimeSettingValue
  value: RuntimeSettingValue
  source: RuntimeSettingSource
  env: string
  apply: RuntimeSettingApply
  risk: RuntimeSettingRisk
  onboarding: boolean
  /** Set for an int setting limited to a fixed list of values. */
  allowed_values?: number[]
  pending_restart: boolean
  updated_at?: string
  updated_by?: string
}

export interface RuntimeRestartInfo {
  supported: boolean
  pending_restart: boolean
}

export interface RuntimeSettingsResponse {
  settings: RuntimeSetting[]
  restart: RuntimeRestartInfo
}

export interface RuntimeOnboardingResponse {
  items: RuntimeSetting[]
  reviewed: boolean
}

/** PUT body value: ints travel as strings, lists as arrays. */
export type RuntimeSettingWrite = boolean | string | string[]
