import type { GitHubAppInstallation } from '../types/githubApp'
import type { GitProviderConnection } from '../types/gitConnections'
import type { GitRepoConnectionFacet } from '../types/gitRepos'
import type { GitProviderName, GitProviderStatus } from '../types/gitProviders'

export type GitAccountHealth =
  'ok' | 'needs_reconnect' | 'not_authorized' | 'rate_limited'

export interface GitAccount {
  key: string
  provider: GitProviderName
  name: string
  label: string
  host?: string
  health: GitAccountHealth
  detail?: string
  scopes?: string
  repos?: number
  apps?: number
  isDefault: boolean
  settingsPath: GitSettingsPath
  authorizePath?: string
  connectionId?: string
  githubInstallationId?: number
}

export const GIT_HOSTS: readonly GitProviderName[] = [
  'github',
  'gitlab',
  'bitbucket',
  'bitbucket_server',
  'gitea',
  'forgejo',
]

export type GitSettingsPath = (typeof SETTINGS_PATH)[GitProviderName]

const SETTINGS_PATH = {
  github: '/settings/github-app',
  gitlab: '/settings/gitlab-app',
  bitbucket: '/settings/bitbucket-app',
  bitbucket_server: '/settings/bitbucket-server-app',
  gitea: '/settings/gitea-app',
  forgejo: '/settings/forgejo-app',
} as const

export function gitSettingsPath(provider: GitProviderName): GitSettingsPath {
  return SETTINGS_PATH[provider]
}

// Bitbucket Server signs in with a token, so it needs no OAuth callback URL.
export function hostNeedsPublicDomain(provider: GitProviderName): boolean {
  return provider !== 'bitbucket_server'
}

export function mapHealth(
  authorized: boolean,
  status?: string,
): GitAccountHealth {
  if (!authorized) return 'not_authorized'
  if (status === 'needs_reauthorize' || status === 'revoked') {
    return 'needs_reconnect'
  }
  if (status === 'rate_limited') return 'rate_limited'
  return 'ok'
}

export interface GitAccountInputs {
  providers: GitProviderStatus[]
  connections: GitProviderConnection[]
  installations: GitHubAppInstallation[]
  facets: GitRepoConnectionFacet[]
}

export function buildGitAccounts({
  providers,
  connections,
  installations,
  facets,
}: GitAccountInputs): GitAccount[] {
  const github = providers.find((p) => p.provider === 'github')
  const githubHealth = mapHealth(true, github?.connection?.status)
  const fromGitHub = installations.map<GitAccount>((i) => {
    const facet = facets.find(
      (f) =>
        f.provider === 'github' &&
        f.account?.toLowerCase() === i.account_login.toLowerCase(),
    )
    return {
      key: `github:${i.installation_id}`,
      provider: 'github',
      name: i.account_login,
      label: i.account_login,
      host: 'github.com',
      health: githubHealth,
      detail: github?.connection?.detail,
      repos: facet?.loaded,
      isDefault: false,
      settingsPath: SETTINGS_PATH.github,
      githubInstallationId: i.id,
    }
  })
  const fromConnections = connections.map<GitAccount>((c) => ({
    key: c.id,
    provider: c.provider,
    name: c.account_name || c.label,
    label: c.label,
    host: c.host,
    health: mapHealth(c.authorized, c.connection.status),
    detail: c.connection.detail,
    scopes: c.scopes,
    repos: facets.find((f) => f.connection_id === c.id)?.loaded,
    apps: c.source_count,
    isDefault: c.is_default,
    settingsPath: SETTINGS_PATH[c.provider],
    authorizePath: c.authorize_path || undefined,
    connectionId: c.id,
  }))
  return [...fromGitHub, ...fromConnections].sort(
    (a, b) =>
      GIT_HOSTS.indexOf(a.provider) - GIT_HOSTS.indexOf(b.provider) ||
      a.name.localeCompare(b.name),
  )
}

export function groupByHost(
  accounts: GitAccount[],
): { provider: GitProviderName; accounts: GitAccount[] }[] {
  return GIT_HOSTS.map((provider) => ({
    provider,
    accounts: accounts.filter((a) => a.provider === provider),
  })).filter((g) => g.accounts.length > 0)
}

export function countNeedingAttention(accounts: GitAccount[]): number {
  return accounts.filter((a) => a.health !== 'ok').length
}

export function bannerSize(accounts: GitAccount[]): 'hero' | 'compact' {
  return accounts.length === 0 ? 'hero' : 'compact'
}

const PREF_PREFIX = 'gitAccountFilter.v1'

function prefKey(user: string, provider: string): string {
  return `${PREF_PREFIX}:${user || 'anon'}:${provider}`
}

export function readAccountFilter(user: string, provider: string): string {
  try {
    return window.localStorage.getItem(prefKey(user, provider)) ?? ''
  } catch {
    return ''
  }
}

export function writeAccountFilter(
  user: string,
  provider: string,
  connectionId: string,
): void {
  try {
    if (connectionId === '') {
      window.localStorage.removeItem(prefKey(user, provider))
    } else {
      window.localStorage.setItem(prefKey(user, provider), connectionId)
    }
  } catch {
    // storage unavailable: the choice just is not remembered
  }
}

// A remembered account that was since disconnected falls back to all accounts.
export function resolveAccountFilter(
  stored: string,
  facets: { connection_id: string }[],
): string {
  return facets.some((f) => f.connection_id === stored) ? stored : ''
}
