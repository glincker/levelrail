import type { GitRepoSourceValue } from '../components/GitRepoSourcePicker'
import type { GitRepoItem, GitRepoProvider } from '../types/gitRepos'

// The one shape every provider's repo list adapts into before reaching
// GitRepoCard. A new provider just needs one more case in fromGitRepoItem.
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
export function normalizeRepoUrl(url: string): string {
  return url
    .trim()
    .toLowerCase()
    .replace(/^(https?:\/\/|git@)/, '')
    .replace(':', '/')
    .replace(/\.git$/, '')
    .replace(/\/$/, '')
}

// fromGitRepoItem adapts one row of the provider-neutral listing
// (GET /api/v1/git/repos) into what the picker and the use-as-source calls need.
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
  switch (provider) {
    case 'github':
      return {
        ...base,
        providerRef: { kind: 'github', owner: item.owner, repo: item.name },
      }
    case 'gitlab':
      return {
        ...base,
        providerRef: {
          kind: 'gitlab',
          connectionId,
          repo: String(item.project_id ?? 0),
        },
      }
    case 'bitbucket':
    case 'gitea':
      return {
        ...base,
        providerRef: { kind: provider, connectionId, repo: item.full_name },
      }
  }
}
