import type { BrandIconName } from '../components/BrandIcon'

// Icon matching only, never a trust decision: the parsed hostname has to
// equal a known host, so "github.com.evil.test" gets no icon.
const GIT_HOST_ICONS: Record<string, BrandIconName> = {
  'github.com': 'github',
  'gitlab.com': 'gitlab',
  'bitbucket.org': 'bitbucket',
}

// gitHostname returns the host of a URL (scheme optional) or an
// scp-style "git@host:owner/repo" remote, or null when it cannot parse.
export function gitHostname(repoUrl: string): string | null {
  const value = repoUrl.trim()
  const scp = /^[\w.-]+@([\w.-]+):(?!\/)/.exec(value)
  if (scp?.[1]) return scp[1].toLowerCase()
  const withScheme = /^[a-z][a-z0-9+.-]*:\/\//i.test(value)
    ? value
    : `https://${value}`
  try {
    return new URL(withScheme).hostname.toLowerCase() || null
  } catch {
    return null
  }
}

// gitHostIconName picks the brand mark for a known public git host,
// null for anything else (including self-hosted instances).
export function gitHostIconName(repoUrl: string): BrandIconName | null {
  const host = gitHostname(repoUrl)?.replace(/^www\./, '')
  return host ? (GIT_HOST_ICONS[host] ?? null) : null
}

export type GitConnectProvider =
  | 'gitlab'
  | 'gitea'
  | 'forgejo'
  | 'bitbucket'
  | 'bitbucket_server'
  | 'azure_devops'

export interface GitConnectConfig {
  /** False for hosts that only connect with a personal access token. */
  oauth: boolean
  /** Fixed host for SaaS providers, empty when the user supplies a base URL. */
  defaultBase: string
  /** Permissions to tick on the OAuth application, as the host labels them. */
  scopes: readonly string[]
  /** The page on the provider where an OAuth application is registered. */
  appPage: ((baseUrl: string) => string) | null
  /** Shape of a typical client id and secret. A mismatch only warns. */
  idPattern: RegExp | null
  secretPattern: RegExp | null
}

const trimBase = (url: string) => url.trim().replace(/\/+$/, '')
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

// One row per provider drives the guided connect panel. Patterns are hints
// from the providers' usual output, never a gate, since versions differ.
export const GIT_CONNECT_CONFIG: Record<GitConnectProvider, GitConnectConfig> =
  {
    gitlab: {
      oauth: true,
      defaultBase: 'https://gitlab.com',
      scopes: ['api'],
      appPage: (base) => `${trimBase(base)}/-/user_settings/applications`,
      idPattern: /^[0-9a-f]{64}$/i,
      secretPattern: /^(gloas-)?[0-9a-f]{64}$/i,
    },
    gitea: {
      oauth: true,
      defaultBase: '',
      scopes: [],
      appPage: (base) => `${trimBase(base)}/user/settings/applications`,
      idPattern: UUID,
      secretPattern: /^gto_[a-z0-9]{20,}$/i,
    },
    forgejo: {
      oauth: true,
      defaultBase: 'https://codeberg.org',
      scopes: [],
      appPage: (base) => `${trimBase(base)}/user/settings/applications`,
      idPattern: UUID,
      secretPattern: /^gto_[a-z0-9]{20,}$/i,
    },
    bitbucket: {
      oauth: true,
      defaultBase: 'https://bitbucket.org',
      scopes: [
        'Account: Read',
        'Repositories: Read, Write',
        'Pull requests: Read, Write',
        'Webhooks: Read and write',
      ],
      appPage: () => 'https://bitbucket.org/account/workspaces/',
      idPattern: /^[A-Za-z0-9]{18}$/,
      secretPattern: /^[A-Za-z0-9_-]{32}$/,
    },
    bitbucket_server: {
      oauth: false,
      defaultBase: '',
      scopes: [],
      appPage: null,
      idPattern: null,
      secretPattern: null,
    },
    azure_devops: {
      oauth: false,
      defaultBase: 'https://dev.azure.com/',
      scopes: [],
      appPage: null,
      idPattern: null,
      secretPattern: null,
    },
  }

// The control plane route the provider redirects back to after approval.
export function gitRedirectUri(
  origin: string,
  provider: GitConnectProvider,
): string {
  return `${trimBase(origin)}/api/v1/${provider}-app/callback`
}

// Link to the provider's OAuth application page, null when the base URL is
// not a usable http(s) address or the provider has no such page.
export function gitAppPageUrl(
  provider: GitConnectProvider,
  baseUrl: string,
): string | null {
  const page = GIT_CONNECT_CONFIG[provider].appPage
  if (!page) return null
  const base = trimBase(baseUrl) || GIT_CONNECT_CONFIG[provider].defaultBase
  if (!/^https?:\/\/[^\s/]+/i.test(base)) return null
  return page(base)
}

export type RedirectHint = 'https' | 'loopback' | 'insecure'

// Providers refuse plain http redirects except to loopback, so a plain http
// install is told up front to use a token instead.
export function gitRedirectHint(origin: string): RedirectHint {
  try {
    const url = new URL(origin)
    if (url.protocol === 'https:') return 'https'
    const host = url.hostname
    if (host === 'localhost' || host === '127.0.0.1' || host === '[::1]') {
      return 'loopback'
    }
  } catch {
    return 'insecure'
  }
  return 'insecure'
}

export type CredentialIssue = 'empty' | 'whitespace' | 'unusual'

// Format and emptiness only. The value is never echoed back, and "unusual"
// is a warning that must not block saving.
export function gitCredentialIssue(
  provider: GitConnectProvider,
  kind: 'id' | 'secret',
  value: string,
): CredentialIssue | null {
  if (value.trim() === '') return 'empty'
  if (/\s/.test(value.trim())) return 'whitespace'
  const config = GIT_CONNECT_CONFIG[provider]
  const pattern = kind === 'id' ? config.idPattern : config.secretPattern
  return pattern && !pattern.test(value.trim()) ? 'unusual' : null
}
