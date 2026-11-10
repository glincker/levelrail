// Mirrors the server allowlists so the form explains a bad value before saving.
// The server stays the authority and validates again.

export interface BuildSettings {
  builder: string
  dockerfile_path: string
  target: string
  no_cache: boolean
  provider: string
  has_git_source: boolean
  providers: string[]
  max_value_bytes: number
  updated_at?: string
}

export interface BuildSettingsDraft {
  builder: string
  dockerfile_path: string
  target: string
  no_cache: boolean
  provider: string
}

export type BuildSettingsPayload = Partial<BuildSettingsDraft>

export type BuildSettingsError =
  'tooLong' | 'targetCharset' | 'pathRelative' | 'provider'

export const BUILDERS = ['dockerfile', 'railpack', 'static'] as const

const TARGET_PATTERN = /^[A-Za-z0-9][A-Za-z0-9._-]*$/

export function draftFrom(s: BuildSettings): BuildSettingsDraft {
  return {
    builder: s.builder,
    dockerfile_path: s.dockerfile_path,
    target: s.target,
    no_cache: s.no_cache,
    provider: s.provider,
  }
}

export function validateDraft(
  d: BuildSettingsDraft,
  s: BuildSettings,
): Partial<Record<keyof BuildSettingsDraft, BuildSettingsError>> {
  const errors: Partial<Record<keyof BuildSettingsDraft, BuildSettingsError>> =
    {}
  const max = s.max_value_bytes
  const path = d.dockerfile_path.trim()
  const target = d.target.trim()
  if (path.length > max) errors.dockerfile_path = 'tooLong'
  else if (
    path.startsWith('/') ||
    path.split('/').some((part) => part === '..')
  )
    errors.dockerfile_path = 'pathRelative'
  if (target.length > max) errors.target = 'tooLong'
  else if (target !== '' && !TARGET_PATTERN.test(target))
    errors.target = 'targetCharset'
  if (d.provider !== '' && !s.providers.includes(d.provider))
    errors.provider = 'provider'
  return errors
}

// Only changed keys go out, so the server audit row names exactly what changed.
export function savePayload(
  d: BuildSettingsDraft,
  s: BuildSettings,
): BuildSettingsPayload {
  const out: BuildSettingsPayload = {}
  if (d.target.trim() !== s.target) out.target = d.target.trim()
  if (d.provider !== s.provider) out.provider = d.provider
  if (d.no_cache !== s.no_cache) out.no_cache = d.no_cache
  if (s.has_git_source) {
    if (d.builder !== s.builder) out.builder = d.builder
    if (d.dockerfile_path.trim() !== s.dockerfile_path)
      out.dockerfile_path = d.dockerfile_path.trim()
  }
  return out
}
