import { gitRedirectHint, type GitConnectProvider } from './gitHost'
import type { GitAccessInfo } from '../types/gitAccess'
import type { GitProviderConnection } from '../types/gitConnections'

export type AccessMethodId = 'oauth' | 'token' | 'deploy_token' | 'ci_job'

// Which ways in each provider offers. Deploy tokens and CI job tokens are
// GitLab features with a matching page here; the rest connect by OAuth or token.
export const GIT_ACCESS_METHODS: Record<GitConnectProvider, AccessMethodId[]> =
  {
    gitlab: ['oauth', 'token', 'deploy_token', 'ci_job'],
    gitea: ['oauth', 'token'],
    forgejo: ['oauth', 'token'],
    bitbucket: ['oauth', 'token'],
    bitbucket_server: ['token'],
    azure_devops: ['token'],
  }

export type CapabilityId =
  | 'list_repos'
  | 'clone_sync'
  | 'register_webhook'
  | 'tag_push_trigger'
  | 'commit_status'
  | 'mr_comments'
  | 'deployments_report'
  | 'registry_pull'
  | 'import_group'

export const CAPABILITY_IDS: readonly CapabilityId[] = [
  'list_repos',
  'clone_sync',
  'register_webhook',
  'tag_push_trigger',
  'commit_status',
  'mr_comments',
  'deployments_report',
  'registry_pull',
  'import_group',
]

// Each capability lists alternatives; holding any one alternative allows it.
type ScopeMap = Partial<Record<CapabilityId, string[]>>

const GITLAB_WRITE = ['api']
export const CAPABILITY_SCOPES: Record<GitConnectProvider, ScopeMap> = {
  gitlab: {
    list_repos: ['api', 'read_api'],
    clone_sync: ['api', 'read_repository'],
    register_webhook: GITLAB_WRITE,
    tag_push_trigger: GITLAB_WRITE,
    commit_status: GITLAB_WRITE,
    mr_comments: GITLAB_WRITE,
    deployments_report: GITLAB_WRITE,
    registry_pull: ['read_registry'],
    import_group: ['api', 'read_api'],
  },
  gitea: {
    list_repos: ['read:repository', 'write:repository'],
    clone_sync: ['read:repository', 'write:repository'],
    register_webhook: ['write:repository'],
    tag_push_trigger: ['write:repository'],
    commit_status: ['write:repository'],
    mr_comments: ['write:issue', 'write:repository'],
  },
  forgejo: {
    list_repos: ['read:repository', 'write:repository'],
    clone_sync: ['read:repository', 'write:repository'],
    register_webhook: ['write:repository'],
    tag_push_trigger: ['write:repository'],
    commit_status: ['write:repository'],
    mr_comments: ['write:issue', 'write:repository'],
  },
  bitbucket: {
    list_repos: ['repository', 'repository:write'],
    clone_sync: ['repository', 'repository:write'],
    register_webhook: ['webhook'],
    tag_push_trigger: ['webhook'],
    commit_status: ['repository:write'],
    mr_comments: ['pullrequest:write'],
  },
  bitbucket_server: {},
  azure_devops: {},
}

// Features a provider cannot do at all, whatever the credential holds.
export const UNSUPPORTED: Partial<Record<GitConnectProvider, CapabilityId[]>> =
  { bitbucket_server: ['mr_comments'] }

export type CapabilityState =
  | { kind: 'allowed' }
  | { kind: 'needs_scope'; scope: string }
  | { kind: 'not_allowed' }
  | { kind: 'unknown' }

// Scopes arrive as one space or comma separated string on the connection.
export function parseScopes(raw: string | undefined): string[] | null {
  if (raw === undefined || raw.trim() === '') return null
  return raw.split(/[\s,]+/).filter(Boolean)
}

export function capabilityState(
  provider: GitConnectProvider,
  id: CapabilityId,
  granted: string[] | null,
): CapabilityState {
  if (UNSUPPORTED[provider]?.includes(id)) return { kind: 'not_allowed' }
  const wanted = CAPABILITY_SCOPES[provider][id]
  if (!wanted || granted === null) return { kind: 'unknown' }
  if (wanted.some((s) => granted.includes(s))) return { kind: 'allowed' }
  return { kind: 'needs_scope', scope: wanted[0] ?? '' }
}

// Days until expiry, null when no date is known or it is not parseable.
export function daysUntil(iso: string | null | undefined, now: number) {
  if (!iso) return null
  const at = Date.parse(iso)
  return Number.isNaN(at) ? null : Math.floor((at - now) / 86_400_000)
}

export const EXPIRY_WARN_DAYS = Number(
  import.meta.env.VITE_GIT_TOKEN_EXPIRY_WARN_DAYS ?? 14,
)

export type MethodStatus =
  | 'connected'
  | 'not_connected'
  | 'needs_attention'
  | 'expires_soon'
  | 'separate'

// Status comes only from the connections the API returns. Methods that live
// on another page report no status instead of a guess.
export function methodStatus(
  method: AccessMethodId,
  connections: GitProviderConnection[],
  info: GitAccessInfo | undefined,
  now: number,
): MethodStatus {
  if (method === 'deploy_token' || method === 'ci_job') return 'separate'
  const mine = connections.filter((c) =>
    method === 'token' ? c.auth_method === 'token' : c.auth_method !== 'token',
  )
  if (mine.length === 0) return 'not_connected'
  if (
    mine.some(
      (c) =>
        !c.authorized ||
        c.connection.status === 'needs_reauthorize' ||
        c.connection.status === 'revoked',
    )
  ) {
    return 'needs_attention'
  }
  const left = daysUntil(info?.expires_at, now)
  if (left !== null && left <= EXPIRY_WARN_DAYS) return 'expires_soon'
  return 'connected'
}

// Recommended: OAuth on an HTTPS install, a token everywhere else, so a plain
// HTTP or private install is never pointed at a flow it cannot finish.
export function recommendedMethod(
  provider: GitConnectProvider,
  origin: string,
): AccessMethodId {
  const methods = GIT_ACCESS_METHODS[provider]
  if (!methods.includes('oauth')) return 'token'
  return gitRedirectHint(origin) === 'insecure' ? 'token' : 'oauth'
}
