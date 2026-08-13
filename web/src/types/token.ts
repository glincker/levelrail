// Wire types for the API token resource, matching internal/api/tokens.go's
// tokenResource / createTokenResponse and internal/api/abilities.go's
// ability strings exactly (see that file's validAbilities). Session-only
// endpoints (POST/GET/DELETE /api/v1/auth/tokens[/{id}]): a bearer token
// can never mint or list or revoke another token, per tokens.go's own doc
// comment on handleCreateToken, so there is no token-authenticated path
// through this file, only the session-authenticated dashboard.

export type Ability =
  | 'read'
  | 'read:sensitive'
  | 'write'
  | 'write:sensitive'
  | 'deploy'
  | 'root'

// Ordered to match abilities.go's validAbilities declaration order, which
// CreateTokenDialog's checkbox list renders in directly.
export const ABILITIES: Ability[] = [
  'read',
  'read:sensitive',
  'write',
  'write:sensitive',
  'deploy',
  'root',
]

// Matches tokenResource exactly: `token` (the plaintext secret) is
// deliberately absent here, it only ever appears on
// CreateTokenResponse, and only in the one response that mints it.
export interface TokenResource {
  id: string
  name: string
  abilities: Ability[]
  created_at: string
  last_used_at?: string
  expires_at?: string
  revoked_at?: string
}

export interface CreateTokenRequest {
  name: string
  abilities: Ability[]
  // Omitted entirely (not 0, not null) means "never expires", matching
  // createTokenRequest's `omitempty` json tag on the Go side.
  expires_in_days?: number
}

// Matches createTokenResponse: tokenResource's fields plus the plaintext
// `token`, present only in this one response shape, never in
// TokenResource / the list endpoint.
export interface CreateTokenResponse extends TokenResource {
  token: string
}
