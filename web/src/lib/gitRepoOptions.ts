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
}

export function fromGitHubRepo(repo: GitHubAppRepo): NormalizedRepoOption {
  return {
    key: repo.full_name,
    displayPath: repo.full_name,
    private: repo.private,
    defaultBranch: repo.default_branch,
    cloneUrl: repo.clone_url,
    providerRef: { kind: 'github', owner: repo.owner_login, repo: repo.name },
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
