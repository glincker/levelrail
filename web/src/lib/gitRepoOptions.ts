import type { GitHubAppRepo } from '../types/githubApp'
import type { GitLabAppProject } from '../types/gitlabApp'
import type { BitbucketAppRepo } from '../types/bitbucketApp'
import type { GiteaAppRepo } from '../types/giteaApp'
import type { GitRepoSourceValue } from '../components/GitRepoSourcePicker'
import type { GitRepoItem, GitRepoProvider } from '../types/gitRepos'

// The one shape every provider's repo list adapts into before reaching
// RepoPickerGrid/GitRepoCard. A new provider just needs one more
// `fromXRepo` adapter here.
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
  archived?: boolean
  fork?: boolean
}

export function fromGitHubRepo(repo: GitHubAppRepo): NormalizedRepoOption {
  return {
    key: repo.full_name,
    displayPath: repo.full_name,
    private: repo.private,
    defaultBranch: repo.default_branch,
    cloneUrl: repo.clone_url,
    providerRef: { kind: 'github', owner: repo.owner_login, repo: repo.name },
    ownerLogin: repo.owner_login,
    accountType: repo.account_type,
    language: repo.language,
    pushedAt: repo.pushed_at,
  }
}

export function fromGitLabProject(
  project: GitLabAppProject,
): NormalizedRepoOption {
  return {
    key: String(project.id),
    displayPath: project.path_with_namespace,
    private: project.visibility !== 'public',
    defaultBranch: project.default_branch,
    cloneUrl: project.clone_url,
    providerRef: { kind: 'gitlab', projectId: project.id },
  }
}

export function fromBitbucketRepo(
  repo: BitbucketAppRepo,
): NormalizedRepoOption {
  const [workspace, repoSlug] = repo.full_name.split('/')
  return {
    key: repo.full_name,
    displayPath: repo.full_name,
    private: repo.private,
    defaultBranch: repo.default_branch,
    cloneUrl: repo.clone_url,
    providerRef: {
      kind: 'bitbucket',
      workspace: workspace ?? '',
      repoSlug: repoSlug ?? '',
    },
  }
}

export function fromGiteaRepo(repo: GiteaAppRepo): NormalizedRepoOption {
  const [owner, repoName] = repo.full_name.split('/')
  return {
    key: repo.full_name,
    displayPath: repo.full_name,
    private: repo.private,
    defaultBranch: repo.default_branch,
    cloneUrl: repo.clone_url,
    providerRef: { kind: 'gitea', owner: owner ?? '', repo: repoName ?? '' },
  }
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
  const slash = item.full_name.lastIndexOf('/')
  const parent = slash >= 0 ? item.full_name.slice(0, slash) : ''
  const leaf = slash >= 0 ? item.full_name.slice(slash + 1) : item.full_name
  const base = {
    key: item.key,
    displayPath: item.full_name,
    private: item.private,
    defaultBranch: item.default_branch,
    cloneUrl: item.clone_url,
    ownerLogin: provider === 'github' ? item.owner : undefined,
    accountType: item.account_type,
    language: item.language,
    pushedAt: item.pushed_at,
    owner: item.owner,
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
        providerRef: { kind: 'gitlab', projectId: item.project_id ?? 0 },
      }
    case 'bitbucket':
      return {
        ...base,
        providerRef: { kind: 'bitbucket', workspace: parent, repoSlug: leaf },
      }
    case 'gitea':
      return {
        ...base,
        providerRef: { kind: 'gitea', owner: parent, repo: leaf },
      }
  }
}
