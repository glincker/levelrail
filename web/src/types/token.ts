// Wire types for the API token resource, matching internal/api/tokens.go's
// tokenResource / createTokenResponse and internal/api/abilities.go's
// ability strings exactly (see that file's validAbilities). Session-only
// endpoints (POST/GET/DELETE /api/v1/auth/tokens[/{id}]): a bearer token
// can never mint or list or revoke another token, per tokens.go's own doc
// comment on handleCreateToken, so there is no token-authenticated path
// through this file, only the session-authenticated dashboard.

export type Ability =
  'read' | 'read:sensitive' | 'write' | 'write:sensitive' | 'deploy' | 'root'

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

// Shared badge coloring for an ability chip: TokenTable (a token's own
// Abilities) and UserTable (a user's own Abilities) both render the same
// strings, so they share one variant map instead of each picking colors
// independently.
export const ABILITY_BADGE_VARIANT: Record<
  Ability,
  'default' | 'outline' | 'destructive' | 'muted'
> = {
  read: 'outline',
  'read:sensitive': 'outline',
  write: 'default',
  'write:sensitive': 'default',
  deploy: 'default',
  root: 'destructive',
}

// Ability picker copy, one line each: a small, legible permission
// surface, not a permission matrix. Shared by
// CreateTokenDialog (an api_tokens row), CreateUserDialog, and
// EditUserAbilitiesDialog (a users row) via components/AbilitiesField.tsx:
// internal/api/abilities.go's validateAbilities is the same function for
// both, so the picker's data lives in one place, not two. Kept in this
// plain data file, not the component file, so AbilitiesField.tsx only
// ever exports a component (react-refresh/only-export-components).
export const ABILITY_OPTIONS: { value: Ability; label: string; hint: string }[] = [
  {
    value: 'read',
    label: 'Read',
    hint: 'List and view apps, deploys, domains.',
  },
  {
    value: 'read:sensitive',
    label: 'Read sensitive',
    hint: 'Read env vars and other sensitive fields.',
  },
  {
    value: 'write',
    label: 'Write',
    hint: 'Edit app config, domains, env vars.',
  },
  {
    value: 'write:sensitive',
    label: 'Write sensitive',
    hint: 'Set secret values. Separate from Write: a token scoped to plain Write cannot touch secrets.',
  },
  { value: 'deploy', label: 'Deploy', hint: 'Trigger deploys and rollbacks.' },
  {
    value: 'root',
    label: 'Root',
    hint: 'Everything below. Exclusive of every other ability.',
  },
]

// Mirrors abilities.go's own validateAbilities exclusivity rule:
// selecting root clears every other checkbox;
// selecting any other checkbox clears root if it was set. Root is never
// left combined with anything else, matching what the server would
// reject anyway, rather than letting the UI produce an invalid
// combination for validateAbilities to bounce back as a 400.
export function toggleAbility(current: Ability[], ability: Ability): Ability[] {
  if (ability === 'root') {
    return current.includes('root') ? [] : ['root']
  }
  const withoutRoot = current.filter((a) => a !== 'root')
  return withoutRoot.includes(ability)
    ? withoutRoot.filter((a) => a !== ability)
    : [...withoutRoot, ability]
}

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
