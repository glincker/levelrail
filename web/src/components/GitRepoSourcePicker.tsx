import { useMemo, useState } from 'react'
import { Link } from '@tanstack/react-router'
import {
  GithubLogoIcon,
  GitlabLogoIcon,
  GitBranchIcon,
  LinkIcon,
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
import {
  useGitHubAppBranches,
  useGitHubAppRepos,
  useGitHubAppStatus,
} from '../queries/githubApp'
import { useGitLabAppProjects, useGitLabAppStatus } from '../queries/gitlabApp'
import {
  useBitbucketAppBranches,
  useBitbucketAppRepos,
  useBitbucketAppStatus,
} from '../queries/bitbucketApp'
import type { GitLabAppProject } from '../types/gitlabApp'
import type { BitbucketAppRepo } from '../types/bitbucketApp'

// GitRepoSourcePicker is the shared "step 1" of both CreateAppFromGitFields
// (this PR) and, per the connect-UX unification proposal's PR 3, a future
// GitSourceCard: one list, one mental model, for all three connected git
// providers plus a manual paste-a-URL fallback, instead of a different
// widget per entry point. See docs-local/research/git-provider-connect-ux-
// unification-proposal.md sections 2 and 4 for the full design and the two
// bugs this closes: the GitHub wizard path never created a git_source row
// (no continuous deployment), and the GitLab/Bitbucket settings-page paths
// never triggered a first build (nothing running until a manual push).
//
// GitLab has no branch-listing API yet (proposal section 3), so its row is
// a free-text branch field prefilled with the project's default_branch
// rather than a select; GitHub and Bitbucket both support branch listing
// and get a real Select. Adding GitLab branch listing is explicitly out of
// scope here (proposal PR 2).
export type GitRepoSourceProvider = 'github' | 'gitlab' | 'bitbucket' | 'manual'

// providerRef carries what the gitlab/bitbucket "use as source" endpoints
// need beyond a plain clone URL (a numeric project id, or a workspace +
// repo slug pair): see connectGitLabProjectAsSource/
// connectBitbucketRepoAsSource in queries/gitlabApp.ts and
// queries/bitbucketApp.ts. GitHub and manual picks have no such endpoint to
// call yet (proposal section 3: GitHub's own use-as-source route doesn't
// exist), so both degrade to the generic PUT .../git-source call instead,
// which needs nothing beyond repoUrl/branch/token.
export interface GitRepoSourceValue {
  provider: GitRepoSourceProvider
  repoUrl: string
  branch: string
  /** Manual mode's optional deploy token for a private pasted repo. Never
   *  set for a provider pick: those authenticate through the provider
   *  connection itself, not a caller-supplied token. */
  token?: string
  providerRef?:
    | { kind: 'gitlab'; projectId: number }
    | { kind: 'bitbucket'; workspace: string; repoSlug: string }
}

function ProviderStatusRow({
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

function GitHubProviderRow({
  disabled,
  onSelect,
}: {
  disabled?: boolean
  onSelect: (value: GitRepoSourceValue) => void
}) {
  const { data: status } = useGitHubAppStatus()
  const enabled = status.connected && status.installed
  const repos = useGitHubAppRepos(enabled)
  const [selectedRepo, setSelectedRepo] = useState('')
  const branches = useGitHubAppBranches(
    selectedRepo.split('/')[0] ?? '',
    selectedRepo.split('/')[1] ?? '',
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
        icon={<GithubLogoIcon className="size-4" />}
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
                    repos.isLoading ? 'Loading repositories...' : 'Select a repository'
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
                  if (repo) {
                    onSelect({ provider: 'github', repoUrl: repo.cloneUrl, branch: ref })
                  }
                }}
                disabled={disabled}
              >
                <SelectTrigger id="git-picker-github-branch" className="w-full">
                  <SelectValue
                    placeholder={branches.isLoading ? 'Loading branches...' : 'Select a branch'}
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
                <p className="text-sm text-destructive">{branches.error.message}</p>
              ) : null}
            </Field>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}

function GitLabProviderRow({
  disabled,
  onSelect,
}: {
  disabled?: boolean
  onSelect: (value: GitRepoSourceValue) => void
}) {
  const { data: status } = useGitLabAppStatus()
  const enabled = status.connected && status.authorized
  const projects = useGitLabAppProjects(enabled)
  const [selectedProject, setSelectedProject] = useState<GitLabAppProject | null>(null)
  const [branch, setBranch] = useState('')

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
        icon={<GitlabLogoIcon className="size-4" />}
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
                const project = (projects.data ?? []).find((p) => String(p.id) === value)
                if (project) selectProject(project)
              }}
              disabled={disabled}
            >
              <SelectTrigger id="git-picker-gitlab-project" className="w-full">
                <SelectValue
                  placeholder={projects.isLoading ? 'Loading projects...' : 'Select a project'}
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
              <p className="text-sm text-destructive">{projects.error.message}</p>
            ) : null}
          </Field>

          {selectedProject ? (
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
                      providerRef: { kind: 'gitlab', projectId: selectedProject.id },
                    })
                  }
                }}
              />
              <p className="text-xs text-muted-foreground">
                GitLab has no branch-listing API here yet, so this is a text field
                prefilled with the project&apos;s default branch.
              </p>
            </Field>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}

function BitbucketProviderRow({
  disabled,
  onSelect,
}: {
  disabled?: boolean
  onSelect: (value: GitRepoSourceValue) => void
}) {
  const { data: status } = useBitbucketAppStatus()
  const enabled = status.connected && status.authorized
  const repos = useBitbucketAppRepos(enabled)
  const [selectedRepo, setSelectedRepo] = useState('')
  const [workspace, repoSlug] = selectedRepo ? selectedRepo.split('/') : ['', '']
  const branches = useBitbucketAppBranches(workspace ?? '', repoSlug ?? '', selectedRepo !== '')

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
        icon={<GitBranchIcon className="size-4" />}
        name="Bitbucket"
        connected={enabled}
        settingsPath="/settings/bitbucket-app"
      />
      {enabled ? (
        <div className="space-y-2 pl-6">
          <Field>
            <FieldLabel htmlFor="git-picker-bitbucket-repo">Repository</FieldLabel>
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
                    repos.isLoading ? 'Loading repositories...' : 'Select a repository'
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
              <FieldLabel htmlFor="git-picker-bitbucket-branch">Branch</FieldLabel>
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
                <SelectTrigger id="git-picker-bitbucket-branch" className="w-full">
                  <SelectValue
                    placeholder={branches.isLoading ? 'Loading branches...' : 'Select a branch'}
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
                <p className="text-sm text-destructive">{branches.error.message}</p>
              ) : null}
            </Field>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}

// ManualSourceRow is the always-visible "Or paste a repository URL" row:
// URL + branch + optional PAT, the same shape GitSourceCard.tsx's own
// manual connect form already uses, reused here rather than reinvented.
// Field ids/labels are deliberately distinct from GitBuildSourceFields'
// own Repository URL / Branch inputs (rendered lower in the same form in
// CreateAppFromGitFields) so the two never collide as duplicate
// accessible names.
function ManualSourceRow({
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
        <LinkIcon className="size-4" />
        Or paste a repository URL
      </div>
      <div className="space-y-2 pl-6">
        <Field>
          <FieldLabel htmlFor="git-picker-manual-url">Paste a repository URL</FieldLabel>
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

// GitRepoSourcePicker itself: three provider rows plus the manual
// fallback, all rendered every time (proposal section 2's "one list, one
// mental model", not a tab switcher). See this file's own doc comment for
// the two bugs this closes.
export function GitRepoSourcePicker({
  disabled,
  onSelect,
}: {
  disabled?: boolean
  /** Called every time a repo+branch becomes fully picked, from any row.
   *  Picking again (a different repo, a different provider, or editing the
   *  manual fields) fires again with the new value; the caller keeps
   *  whichever call came last, the same "last pick wins" model a single
   *  select input would have. */
  onSelect: (value: GitRepoSourceValue) => void
}) {
  const [selected, setSelected] = useState<GitRepoSourceValue | null>(null)

  function handleSelect(value: GitRepoSourceValue) {
    setSelected(value)
    onSelect(value)
  }

  return (
    <div className="space-y-3">
      <div className="space-y-4 rounded-lg border border-dashed border-border p-3">
        <GitHubProviderRow disabled={disabled} onSelect={handleSelect} />
        <div className="border-t border-border" />
        <GitLabProviderRow disabled={disabled} onSelect={handleSelect} />
        <div className="border-t border-border" />
        <BitbucketProviderRow disabled={disabled} onSelect={handleSelect} />
        <div className="border-t border-border pt-2">
          <ManualSourceRow disabled={disabled} onSelect={handleSelect} />
        </div>
      </div>
      {selected ? (
        <p className="text-xs text-muted-foreground">
          Selected: <span className="font-mono">{selected.repoUrl}</span> @{' '}
          <span className="font-mono">{selected.branch}</span>
        </p>
      ) : null}
    </div>
  )
}
