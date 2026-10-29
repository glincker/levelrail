// Wire types for GET /api/v1/apps/{name}/releases (internal/api/app_releases.go).
export interface AppRelease {
  id: string
  image_ref: string
  digest: string
  short_digest: string
  created_at: string
  source_reason: string
}

export interface AppReleasesResponse {
  releases: AppRelease[]
  current_release_id?: string
}
