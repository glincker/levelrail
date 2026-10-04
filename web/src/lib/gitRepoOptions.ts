import type { GitHubAppRepo } from '../types/githubApp'
import type { GitLabAppProject } from '../types/gitlabApp'
import type { BitbucketAppRepo } from '../types/bitbucketApp'
import type { GiteaAppRepo } from '../types/giteaApp'
import type { GitRepoSourceValue } from '../components/GitRepoSourcePicker'

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
  }
}

// GitHubRepoGroup is one connected account's slice of the repo picker's
// card grid: a header (account login, "Personal" for a user account)
// plus that account's own repos, or an error when the installation
// failed to list instead of any repos.
export interface GitHubRepoGroup {
  key: string
  label: string
  options: NormalizedRepoOption[]
  error?: string
}

// groupGitHubRepos preserves installation order (the backend lists
// installations oldest-first): a login appearing in both repos and
// errors (shouldn't happen, each installation either lists or fails)
// keeps its repos and picks up the error too, rather than one silently
// overwriting the other.
export function groupGitHubRepos(
  options: NormalizedRepoOption[],
  errors: { account_login: string; error: string }[],
): GitHubRepoGroup[] {
  const groups = new Map<string, GitHubRepoGroup>()

  function groupFor(
    login: string,
    accountType: 'user' | 'organization' | undefined,
  ): GitHubRepoGroup {
    const existing = groups.get(login)
    if (existing) {
      return existing
    }
    const created: GitHubRepoGroup = {
      key: login,
      label: githubAccountLabel(login, accountType),
      options: [],
    }
    groups.set(login, created)
    return created
  }

  for (const option of options) {
    groupFor(option.ownerLogin ?? '', option.accountType).options.push(option)
  }
  for (const err of errors) {
    groupFor(err.account_login, undefined).error = err.error
  }

  return Array.from(groups.values())
}

function githubAccountLabel(
  login: string,
  accountType: 'user' | 'organization' | undefined,
): string {
  return accountType === 'user' ? `Personal (${login})` : login
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
