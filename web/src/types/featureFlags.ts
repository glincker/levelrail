// Wire types for the feature-flag resource, matching
// internal/api/feature_flags.go's featureFlagResource exactly: same
// field names, same snake_case JSON tags.

// GET/POST/PUT /api/v1/apps/{name}/flags response shape.
export interface FeatureFlag {
  id: string
  key: string
  name: string
  description?: string
  service_name: string
  enabled: boolean
  rollout_percentage: number
  created_at: string
  updated_at: string
}

// POST /api/v1/apps/{name}/flags request body. `key` is read on create
// only; PUT ignores it (the server never lets an update rewrite it).
export interface FeatureFlagRequest {
  key?: string
  name: string
  description?: string
  enabled: boolean
  rollout_percentage: number
}
