import { useMemo, useState } from 'react'
import { Link } from '@tanstack/react-router'
import {
  GithubLogoIcon,
  GitlabLogoIcon,
  GitBranchIcon,
  LinkIcon,
  TeaBagIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { useGitHubAppBranches, useGitHubAppRepos } from '../queries/githubApp'
import {
  useGitLabAppBranches,
  useGitLabAppProjects,
} from '../queries/gitlabApp'
import {
  useBitbucketAppBranches,
  useBitbucketAppRepos,
} from '../queries/bitbucketApp'
import { useGiteaAppBranches, useGiteaAppRepos } from '../queries/giteaApp'
import type { GitLabAppProject } from '../types/gitlabApp'
import type { BitbucketAppRepo } from '../types/bitbucketApp'
import type { GiteaAppRepo } from '../types/giteaApp'
import type { GitProviderStatus } from '../types/gitProviders'
import type { GitRepoSourceValue } from './GitRepoSourcePicker'

// GitRepoSourceRows.tsx: the four provider rows (plus the manual
// fallback) GitRepoSourcePicker.tsx renders, split out purely to keep
// each file under the 500-line cap: extracting one file per row would
// scatter the "one list, one mental model" picker across five files for
// no readability gain, so this groups them instead, the same "extract
// composable components" split CLAUDE.md's frontend rule calls for.

export function ProviderStatusRow({
  icon,
  name,
  connected,
  settingsPath,
}: {
  icon: React.ReactNode
  name: string
  connected: boolean
  settingsPath: string
}) {
  return (
    <div className="flex items-center gap-2 text-sm font-medium text-foreground">
      {icon}
      {name}
      <span className="text-xs font-normal text-muted-foreground">
        {connected ? 'Connected' : 'Not connected'}
      </span>
      {!connected ? (
        <Link
          to={settingsPath}
          className="ml-auto text-xs font-normal text-primary underline underline-offset-2"
        >
          Connect
        </Link>
      ) : null}
    </div>
  )
}

export function GitHubProviderRow({
  provider,
  disabled,
  onSelect,
}: {
  provider: GitProviderStatus
  disabled?: boolean
  onSelect: (value: GitRepoSourceValue) => void
}) {
  const enabled = provider.connected
  const repos = useGitHubAppRepos(enabled)
  const [selectedRepo, setSelectedRepo] = useState('')
  const [owner, repoName] = selectedRepo ? selectedRepo.split('/') : ['', '']
  const branches = useGitHubAppBranches(
    owner ?? '',
    repoName ?? '',
    selectedRepo !== '',
  )

  const repoByFullName = useMemo(() => {
    const map = new Map<string, { cloneUrl: string; defaultBranch: string }>()
    for (const repo of repos.data ?? []) {
      map.set(repo.full_name, {
        cloneUrl: repo.clone_url,
        defaultBranch: repo.default_branch,
      })
    }
    return map
  }, [repos.data])

  return (
    <div className="space-y-2">
      <ProviderStatusRow
        icon={<GithubLogoIcon className="size-4" aria-hidden="true" />}
        name="GitHub"
        connected={enabled}
        settingsPath="/settings/github-app"
      />
      {enabled ? (
        <div className="space-y-2 pl-6">
          <Field>
            <FieldLabel htmlFor="git-picker-github-repo">Repository</FieldLabel>
            <Select
              value={selectedRepo}
              onValueChange={(value) => {
                if (typeof value === 'string') setSelectedRepo(value)
              }}
              disabled={disabled}
            >
              <SelectTrigger id="git-picker-github-repo" className="w-full">
                <SelectValue
                  placeholder={
                    repos.isLoading
                      ? 'Loading repositories...'
                      : 'Select a repository'
                  }
                />
              </SelectTrigger>
              <SelectContent>
                {(repos.data ?? []).map((repo) => (
                  <SelectItem key={repo.full_name} value={repo.full_name}>
                    {repo.full_name}
                    {repo.private ? ' (private)' : ''}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {repos.isError ? (
              <p className="text-sm text-destructive">{repos.error.message}</p>
            ) : null}
          </Field>

          {selectedRepo ? (
            <Field>
              <FieldLabel htmlFor="git-picker-github-branch">Branch</FieldLabel>
              <Select
                onValueChange={(ref) => {
                  if (typeof ref !== 'string') return
                  const repo = repoByFullName.get(selectedRepo)
                  if (repo && owner && repoName) {
                    onSelect({
                      provider: 'github',
                      repoUrl: repo.cloneUrl,
                      branch: ref,
                      providerRef: { kind: 'github', owner, repo: repoName },
                    })
                  }
                }}
                disabled={disabled}
              >
                <SelectTrigger id="git-picker-github-branch" className="w-full">
                  <SelectValue
                    placeholder={
                      branches.isLoading
                        ? 'Loading branches...'
                        : 'Select a branch'
                    }
                  />
                </SelectTrigger>
                <SelectContent>
                  {(branches.data ?? []).map((branch) => (
                    <SelectItem key={branch.name} value={branch.name}>
                      {branch.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {branches.isError ? (
                <p className="text-sm text-destructive">
                  {branches.error.message}
                </p>
              ) : null}
            </Field>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}

export function GitLabProviderRow({
  provider,
  disabled,
  onSelect,
}: {
  provider: GitProviderStatus
  disabled?: boolean
  onSelect: (value: GitRepoSourceValue) => void
}) {
  const enabled = provider.connected
  const projects = useGitLabAppProjects(enabled)
  const [selectedProject, setSelectedProject] =
    useState<GitLabAppProject | null>(null)
  const [branch, setBranch] = useState('')
  const branches = useGitLabAppBranches(
    selectedProject?.id ?? 0,
    provider.can_list_branches && selectedProject !== null,
  )

  function selectProject(project: GitLabAppProject) {
    setSelectedProject(project)
    const initialBranch = project.default_branch
    setBranch(initialBranch)
    onSelect({
      provider: 'gitlab',
      repoUrl: project.clone_url,
      branch: initialBranch,
      providerRef: { kind: 'gitlab', projectId: project.id },
    })
  }

  return (
    <div className="space-y-2">
      <ProviderStatusRow
        icon={<GitlabLogoIcon className="size-4" aria-hidden="true" />}
        name="GitLab"
        connected={enabled}
        settingsPath="/settings/gitlab-app"
      />
      {enabled ? (
        <div className="space-y-2 pl-6">
          <Field>
            <FieldLabel htmlFor="git-picker-gitlab-project">Project</FieldLabel>
            <Select
              value={selectedProject ? String(selectedProject.id) : ''}
              onValueChange={(value) => {
                if (typeof value !== 'string') return
                const project = (projects.data ?? []).find(
                  (p) => String(p.id) === value,
                )
                if (project) selectProject(project)
              }}
              disabled={disabled}
            >
              <SelectTrigger id="git-picker-gitlab-project" className="w-full">
                <SelectValue
                  placeholder={
                    projects.isLoading
                      ? 'Loading projects...'
                      : 'Select a project'
                  }
                />
              </SelectTrigger>
              <SelectContent>
                {(projects.data ?? []).map((project) => (
                  <SelectItem key={project.id} value={String(project.id)}>
                    {project.path_with_namespace}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {projects.isError ? (
              <p className="text-sm text-destructive">
                {projects.error.message}
              </p>
            ) : null}
          </Field>

          {selectedProject && provider.can_list_branches ? (
            <Field>
              <FieldLabel htmlFor="git-picker-gitlab-branch">Branch</FieldLabel>
              <Select
                onValueChange={(ref) => {
                  if (typeof ref !== 'string') return
                  onSelect({
                    provider: 'gitlab',
                    repoUrl: selectedProject.clone_url,
                    branch: ref,
                    providerRef: {
                      kind: 'gitlab',
                      projectId: selectedProject.id,
                    },
                  })
                }}
                disabled={disabled}
              >
                <SelectTrigger id="git-picker-gitlab-branch" className="w-full">
                  <SelectValue
                    placeholder={
                      branches.isLoading
                        ? 'Loading branches...'
                        : 'Select a branch'
                    }
                  />
                </SelectTrigger>
                <SelectContent>
                  {(branches.data ?? []).map((b) => (
                    <SelectItem key={b.name} value={b.name}>
                      {b.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {branches.isError ? (
                <p className="text-sm text-destructive">
                  {branches.error.message}
                </p>
              ) : null}
            </Field>
          ) : null}

          {selectedProject && !provider.can_list_branches ? (
            <Field>
              <FieldLabel htmlFor="git-picker-gitlab-branch">Branch</FieldLabel>
              <Input
                id="git-picker-gitlab-branch"
                className="font-mono"
                autoComplete="off"
                spellCheck={false}
                value={branch}
                disabled={disabled}
                onChange={(e) => {
                  const next = e.target.value
                  setBranch(next)
                  if (next.trim()) {
                    onSelect({
                      provider: 'gitlab',
                      repoUrl: selectedProject.clone_url,
                      branch: next.trim(),
                      providerRef: {
                        kind: 'gitlab',
                        projectId: selectedProject.id,
                      },
                    })
                  }
                }}
              />
              <p className="text-xs text-muted-foreground">
                This GitLab connection doesn&apos;t support branch listing, so
                this is a text field prefilled with the project&apos;s default
                branch.
              </p>
            </Field>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}

export function BitbucketProviderRow({
  provider,
  disabled,
  onSelect,
}: {
  provider: GitProviderStatus
  disabled?: boolean
  onSelect: (value: GitRepoSourceValue) => void
}) {
  const enabled = provider.connected
  const repos = useBitbucketAppRepos(enabled)
  const [selectedRepo, setSelectedRepo] = useState('')
  const [workspace, repoSlug] = selectedRepo
    ? selectedRepo.split('/')
    : ['', '']
  const branches = useBitbucketAppBranches(
    workspace ?? '',
    repoSlug ?? '',
    selectedRepo !== '',
  )

  const repoByFullName = useMemo(() => {
    const map = new Map<string, BitbucketAppRepo>()
    for (const repo of repos.data ?? []) {
      map.set(repo.full_name, repo)
    }
    return map
  }, [repos.data])

  return (
    <div className="space-y-2">
      <ProviderStatusRow
        icon={<GitBranchIcon className="size-4" aria-hidden="true" />}
        name="Bitbucket"
        connected={enabled}
        settingsPath="/settings/bitbucket-app"
      />
      {enabled ? (
        <div className="space-y-2 pl-6">
          <Field>
            <FieldLabel htmlFor="git-picker-bitbucket-repo">
              Repository
            </FieldLabel>
            <Select
              value={selectedRepo}
              onValueChange={(value) => {
                if (typeof value === 'string') setSelectedRepo(value)
              }}
              disabled={disabled}
            >
              <SelectTrigger id="git-picker-bitbucket-repo" className="w-full">
                <SelectValue
                  placeholder={
                    repos.isLoading
                      ? 'Loading repositories...'
                      : 'Select a repository'
                  }
                />
              </SelectTrigger>
              <SelectContent>
                {(repos.data ?? []).map((repo) => (
                  <SelectItem key={repo.full_name} value={repo.full_name}>
                    {repo.full_name}
                    {repo.private ? ' (private)' : ''}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {repos.isError ? (
              <p className="text-sm text-destructive">{repos.error.message}</p>
            ) : null}
          </Field>

          {selectedRepo ? (
            <Field>
              <FieldLabel htmlFor="git-picker-bitbucket-branch">
                Branch
              </FieldLabel>
              <Select
                onValueChange={(ref) => {
                  if (typeof ref !== 'string') return
                  const repo = repoByFullName.get(selectedRepo)
                  if (repo && workspace && repoSlug) {
                    onSelect({
                      provider: 'bitbucket',
                      repoUrl: repo.clone_url,
                      branch: ref,
                      providerRef: { kind: 'bitbucket', workspace, repoSlug },
                    })
                  }
                }}
                disabled={disabled}
              >
                <SelectTrigger
                  id="git-picker-bitbucket-branch"
                  className="w-full"
                >
                  <SelectValue
                    placeholder={
                      branches.isLoading
                        ? 'Loading branches...'
                        : 'Select a branch'
                    }
                  />
                </SelectTrigger>
                <SelectContent>
                  {(branches.data ?? []).map((branch) => (
                    <SelectItem key={branch.name} value={branch.name}>
                      {branch.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {branches.isError ? (
                <p className="text-sm text-destructive">
                  {branches.error.message}
                </p>
              ) : null}
            </Field>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}

export function GiteaProviderRow({
  provider,
  disabled,
  onSelect,
}: {
  provider: GitProviderStatus
  disabled?: boolean
  onSelect: (value: GitRepoSourceValue) => void
}) {
  const enabled = provider.connected
  const repos = useGiteaAppRepos(enabled)
  const [selectedRepo, setSelectedRepo] = useState('')
  const [owner, repoName] = selectedRepo ? selectedRepo.split('/') : ['', '']
  const branches = useGiteaAppBranches(
    owner ?? '',
    repoName ?? '',
    selectedRepo !== '',
  )

  const repoByFullName = useMemo(() => {
    const map = new Map<string, GiteaAppRepo>()
    for (const repo of repos.data ?? []) {
      map.set(repo.full_name, repo)
    }
    return map
  }, [repos.data])

  return (
    <div className="space-y-2">
      <ProviderStatusRow
        icon={<TeaBagIcon className="size-4" aria-hidden="true" />}
        name="Gitea"
        connected={enabled}
        settingsPath="/settings/gitea-app"
      />
      {enabled ? (
        <div className="space-y-2 pl-6">
          <Field>
            <FieldLabel htmlFor="git-picker-gitea-repo">Repository</FieldLabel>
            <Select
              value={selectedRepo}
              onValueChange={(value) => {
                if (typeof value === 'string') setSelectedRepo(value)
              }}
              disabled={disabled}
            >
              <SelectTrigger id="git-picker-gitea-repo" className="w-full">
                <SelectValue
                  placeholder={
                    repos.isLoading
                      ? 'Loading repositories...'
                      : 'Select a repository'
                  }
                />
              </SelectTrigger>
              <SelectContent>
                {(repos.data ?? []).map((repo) => (
                  <SelectItem key={repo.full_name} value={repo.full_name}>
                    {repo.full_name}
                    {repo.private ? ' (private)' : ''}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {repos.isError ? (
              <p className="text-sm text-destructive">{repos.error.message}</p>
            ) : null}
          </Field>

          {selectedRepo ? (
            <Field>
              <FieldLabel htmlFor="git-picker-gitea-branch">Branch</FieldLabel>
              <Select
                onValueChange={(ref) => {
                  if (typeof ref !== 'string') return
                  const repo = repoByFullName.get(selectedRepo)
                  if (repo && owner && repoName) {
                    onSelect({
                      provider: 'gitea',
                      repoUrl: repo.clone_url,
                      branch: ref,
                      providerRef: { kind: 'gitea', owner, repo: repoName },
                    })
                  }
                }}
                disabled={disabled}
              >
                <SelectTrigger id="git-picker-gitea-branch" className="w-full">
                  <SelectValue
                    placeholder={
                      branches.isLoading
                        ? 'Loading branches...'
                        : 'Select a branch'
                    }
                  />
                </SelectTrigger>
                <SelectContent>
                  {(branches.data ?? []).map((branch) => (
                    <SelectItem key={branch.name} value={branch.name}>
                      {branch.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {branches.isError ? (
                <p className="text-sm text-destructive">
                  {branches.error.message}
                </p>
              ) : null}
            </Field>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}

// ManualSourceRow is the always-visible "Or paste a repository URL"
// row: URL + branch + optional PAT, the same shape GitSourceCard.tsx's
// own manual connect form already uses, reused here rather than
// reinvented. Field ids/labels are deliberately distinct from
// GitBuildSourceFields's own Repository URL / Branch inputs (rendered
// lower in the same form in CreateAppFromGitFields) so the two never
// collide as duplicate accessible names.
export function ManualSourceRow({
  disabled,
  onSelect,
}: {
  disabled?: boolean
  onSelect: (value: GitRepoSourceValue) => void
}) {
  const [repoUrl, setRepoUrl] = useState('')
  const [branch, setBranch] = useState('')
  const [token, setToken] = useState('')

  function emit(nextRepoUrl: string, nextBranch: string, nextToken: string) {
    if (nextRepoUrl.trim() && nextBranch.trim()) {
      onSelect({
        provider: 'manual',
        repoUrl: nextRepoUrl.trim(),
        branch: nextBranch.trim(),
        token: nextToken.trim() || undefined,
      })
    }
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2 text-sm font-medium text-foreground">
        <LinkIcon className="size-4" aria-hidden="true" />
        Or paste a repository URL
      </div>
      <div className="space-y-2 pl-6">
        <Field>
          <FieldLabel htmlFor="git-picker-manual-url">
            Paste a repository URL
          </FieldLabel>
          <Input
            id="git-picker-manual-url"
            className="font-mono"
            placeholder="https://github.com/you/app.git"
            autoComplete="off"
            spellCheck={false}
            disabled={disabled}
            value={repoUrl}
            onChange={(e) => {
              setRepoUrl(e.target.value)
              emit(e.target.value, branch, token)
            }}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="git-picker-manual-branch">Branch</FieldLabel>
          <Input
            id="git-picker-manual-branch"
            className="font-mono"
            placeholder="main"
            autoComplete="off"
            spellCheck={false}
            disabled={disabled}
            value={branch}
            onChange={(e) => {
              setBranch(e.target.value)
              emit(repoUrl, e.target.value, token)
            }}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="git-picker-manual-token">
            Deploy token (optional, for a private repo)
          </FieldLabel>
          <Input
            id="git-picker-manual-token"
            type="password"
            autoComplete="off"
            spellCheck={false}
            placeholder="Personal access token"
            disabled={disabled}
            value={token}
            onChange={(e) => {
              setToken(e.target.value)
              emit(repoUrl, branch, e.target.value)
            }}
          />
        </Field>
      </div>
    </div>
  )
}
