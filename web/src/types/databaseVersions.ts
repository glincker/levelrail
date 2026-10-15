export type VersionChannel = 'stable' | 'lts' | 'rc' | 'beta' | 'eol'

export type CatalogOrigin = 'live' | 'published' | 'direct' | 'embedded'

export type CatalogFreshness = 'fresh' | 'ok' | 'aging' | 'stale'

export interface DatabaseVersionInfo {
  version: string
  channel: VersionChannel
  released?: string
  eol?: string
  recommended: boolean
}

export interface DatabaseVersionSource {
  eol_url?: string
  tags_url?: string
  release_notes_url?: string
}

export interface DatabaseVersions {
  engine: string
  origin: CatalogOrigin
  versions: DatabaseVersionInfo[]
  source: DatabaseVersionSource
  generated_at?: string
  fetched_at?: string
  freshness: CatalogFreshness
  message?: string
  cooldown?: boolean
}
