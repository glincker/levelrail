export type OAuthConnectionState = 'connected' | 'disabled' | 'unconfigured'

export interface OAuthStateInput {
  enabled: boolean
  client_id?: string
  has_client_secret: boolean
}

// Credentials saved and sign-in switched on is "connected"; credentials
// saved but switched off is "disabled"; anything else still needs setup.
export function connectionState(s: OAuthStateInput): OAuthConnectionState {
  const hasCredentials = Boolean(s.client_id) && s.has_client_secret
  if (!hasCredentials) return 'unconfigured'
  return s.enabled ? 'connected' : 'disabled'
}

export function callbackUrl(origin: string, provider: string): string {
  const base = origin.replace(/\/+$/, '')
  return `${base}/api/v1/auth/oauth/${encodeURIComponent(provider)}/callback`
}

const BRAND_KEYS: Record<string, string> = {
  google: 'google',
  github: 'github',
  microsoft: 'microsoft',
  oidc: 'openid',
}

export function brandKeyFor(provider: string): string {
  return BRAND_KEYS[provider] ?? provider
}
