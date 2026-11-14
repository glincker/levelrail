import type { GitRepoSourceValue } from '../components/GitRepoSourcePicker'
import type { GitRepoItem, GitRepoProvider } from '../types/gitRepos'

// The one shape every provider's repo list adapts into before reaching
// GitRepoCard. A new provider just needs one more case in fromGitRepoItem;
// the exhaustive check below fails the type check until it has one.
export interface NormalizedRepoOption {
  key: string
  displayPath: string
  private: boolean
  defaultBranch: string
  cloneUrl: string
  providerRef: NonNullable<GitRepoSourceValue['providerRef']>
  /** GitHub only: which connected account this repo came from, for the picker's grouping. */
  ownerLogin?: string
  accountType?: 'user' | 'organization'
  /** GitHub only: primary language and last push time, for the new-app list. */
  language?: string
  pushedAt?: string
  /** Owner or organization, so same-name repos from different owners stay apart. */
  owner?: string
  /** The connected account (and its label) the repository was listed through. */
  account?: string
  connectionId?: string
  connectionLabel?: string
  archived?: boolean
  fork?: boolean
}

// Makes an https and an ssh clone URL comparable for the "running as X" match.
// Tolerates empty and non-string input so a malformed row never throws.
export function normalizeRepoUrl(url: unknown): string {
  if (typeof url !== 'string') return ''
  return (
    url
      .trim()
      .toLowerCase()
      .replace(/^(https?|ssh|git):\/\//, '')
      .replace(/^[^@/]+@/, '')
      // scp style host:owner/repo, but not a host:port/ prefix.
      .replace(/^([^/:]+):(?!\d+\/)/, '$1/')
      .replace(/\.git$/, '')
      .replace(/\/$/, '')
  )
}

type ConnectionRefKind = Exclude<
  NonNullable<GitRepoSourceValue['providerRef']>['kind'],
  'github'
>

// Fails the type check when a GitRepoProvider has no case in fromGitRepoItem.
function assertHandled(provider: never): void {
  void provider
}

/** A provider string from the wire; empty or missing means GitHub, as older rows had no provider. */
export function providerOf(
  item: Pick<GitRepoItem, 'provider'>,
): GitRepoProvider {
  const p: unknown = item.provider
  return typeof p === 'string' && p !== '' ? (p as GitRepoProvider) : 'github'
}

// fromGitRepoItem adapts one row of the provider-neutral listing
// (GET /api/v1/git/repos) into what the picker and the use-as-source calls need.
// It never returns undefined: a provider this build does not know falls back to
// the connection based reference, which the generic use-as-source call accepts.
export function fromGitRepoItem(
  provider: GitRepoProvider,
  item: GitRepoItem,
): NormalizedRepoOption {
  const connectionId = item.connection_id ?? ''
  const base = {
    // The same repository name can come from two accounts, so the key carries the connection.
    key: connectionId ? `${connectionId}:${item.key}` : item.key,
    displayPath: item.full_name,
    private: item.private,
    defaultBranch: item.default_branch,
    cloneUrl: item.clone_url,
    ownerLogin: provider === 'github' ? item.owner : undefined,
    accountType: item.account_type,
    language: item.language,
    pushedAt: item.pushed_at,
    owner: item.owner,
    account: item.account,
    connectionId: item.connection_id,
    connectionLabel: item.connection_label,
    archived: item.archived,
    fork: item.fork,
  }
  const viaConnection = (kind: ConnectionRefKind, repo: string) => ({
    ...base,
    providerRef: { kind, connectionId, repo },
  })
  switch (provider) {
    case 'github':
      return {
        ...base,
        providerRef: { kind: 'github', owner: item.owner, repo: item.name },
      }
    case 'gitlab':
      return viaConnection('gitlab', String(item.project_id ?? 0))
    case 'bitbucket':
    case 'bitbucket_server':
    case 'gitea':
    case 'forgejo':
    case 'azure_devops':
      // Azure DevOps and Bitbucket Server full names read PROJECT/repo, which the
      // connection lookup expects as is.
      return viaConnection(provider, item.full_name)
    default:
      assertHandled(provider)
      // A provider newer than this build: the kind only labels the row.
      return viaConnection(provider, item.full_name)
  }
}

/** Options for every usable row; null, malformed and clone-less rows are skipped, not thrown on. */
export function toRepoOptions(
  items: readonly (GitRepoItem | null | undefined)[] | null | undefined,
  fallbackProvider?: GitRepoProvider,
): NormalizedRepoOption[] {
  const out: NormalizedRepoOption[] = []
  for (const item of items ?? []) {
    if (!item || typeof item !== 'object') continue
    if (typeof item.clone_url !== 'string' || item.clone_url === '') continue
    const provider = item.provider
      ? providerOf(item)
      : (fallbackProvider ?? providerOf(item))
    out.push(fromGitRepoItem(provider, item))
  }
  return out
}
