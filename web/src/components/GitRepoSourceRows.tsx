import { useMemo, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { Combobox } from '@/components/ui/combobox'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { RepoPickerGrid } from './GitRepoPickerGrid'
import {
  fromGitLabProject,
  type NormalizedRepoOption,
} from '../lib/gitRepoOptions'
import {
  useGitLabAppBranches,
  useGitLabAppProjects,
} from '../queries/gitlabApp'
import type { GitProviderStatus } from '../types/gitProviders'
import type { GitRepoSourceValue } from './GitRepoSourcePicker'

// Tab content for GitRepoSourcePicker's five tabs, split across this file
// and GitRepoSimpleProviderRows.tsx to stay under the 500-line cap.
// GitLab is the outlier (numeric id key, auto-selects its default branch,
// free-text branch fallback when can_list_branches is false), so it
// keeps its own body here; GitHub/Bitbucket/Gitea share one in the other
// file. Icon/label/connected-dot live in the TabsTrigger, not here.

export function NotConnectedPrompt({
  name,
  settingsPath,
}: Readonly<{
  name: string
  settingsPath: string
}>) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-lg border border-dashed border-border px-3 py-2.5 text-sm">
      <span className="text-muted-foreground">
        Connect {name} to pick a repository.
      </span>
      <Link
        to={settingsPath}
        className="shrink-0 text-xs font-medium text-primary underline underline-offset-2"
      >
        Connect
      </Link>
    </div>
  )
}

export interface ProviderRowProps {
  provider: GitProviderStatus
  disabled?: boolean
  onSelect: (value: GitRepoSourceValue) => void
  /** Normalized clone URL -> service name, for the "already running as X"
   *  badge. Computed once in GitRepoSourcePicker.tsx and shared across all
   *  four rows, rather than each row fetching it separately. */
  runningRepoByUrl: Map<string, string>
}

export function GitLabProviderRow({
  provider,
  disabled,
  onSelect,
  runningRepoByUrl,
}: ProviderRowProps) {
  const enabled = provider.connected
  const projects = useGitLabAppProjects(enabled)
  const [selectedProjectId, setSelectedProjectId] = useState('')
  const [branch, setBranch] = useState('')

  const options = useMemo(
    () => (projects.data ?? []).map(fromGitLabProject),
    [projects.data],
  )
  const selected = options.find((option) => option.key === selectedProjectId)
  const branches = useGitLabAppBranches(
    selected ? Number(selected.key) : 0,
    provider.can_list_branches && selected !== undefined,
  )
  const branchOptions = useMemo(
    () => (branches.data ?? []).map((b) => ({ value: b.name, label: b.name })),
    [branches.data],
  )

  function selectProject(option: NormalizedRepoOption) {
    setSelectedProjectId(option.key)
    setBranch(option.defaultBranch)
    onSelect({
      provider: 'gitlab',
      repoUrl: option.cloneUrl,
      branch: option.defaultBranch,
      providerRef: option.providerRef,
    })
  }

  if (!enabled) {
    return (
      <NotConnectedPrompt name="GitLab" settingsPath="/settings/gitlab-app" />
    )
  }

  return (
    <div className="space-y-2">
      <Field>
        <FieldLabel htmlFor="git-picker-gitlab-project">Project</FieldLabel>
        <RepoPickerGrid
          searchInputId="git-picker-gitlab-project"
          options={options}
          isLoading={projects.isLoading}
          isError={projects.isError}
          errorMessage={projects.error?.message}
          selectedKey={selectedProjectId}
          disabled={disabled}
          runningRepoByUrl={runningRepoByUrl}
          searchPlaceholder="Search projects..."
          emptyMessage="No projects found."
          onSelect={selectProject}
        />
      </Field>

      {selected && provider.can_list_branches ? (
        <Field>
          <FieldLabel htmlFor="git-picker-gitlab-branch">Branch</FieldLabel>
          <Combobox
            id="git-picker-gitlab-branch"
            options={branchOptions}
            value={branch}
            isLoading={branches.isLoading}
            disabled={disabled}
            placeholder="Select a branch"
            searchPlaceholder="Search branches..."
            onValueChange={(ref) => {
              setBranch(ref)
              onSelect({
                provider: 'gitlab',
                repoUrl: selected.cloneUrl,
                branch: ref,
                providerRef: selected.providerRef,
              })
            }}
          />
          {branches.isError ? (
            <p className="text-sm text-destructive">{branches.error.message}</p>
          ) : null}
        </Field>
      ) : null}

      {selected && !provider.can_list_branches ? (
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
                  repoUrl: selected.cloneUrl,
                  branch: next.trim(),
                  providerRef: selected.providerRef,
                })
              }
            }}
          />
          <p className="text-xs text-muted-foreground">
            This GitLab connection doesn&apos;t support branch listing, so this
            is a text field prefilled with the project&apos;s default branch.
          </p>
        </Field>
      ) : null}
    </div>
  )
}

// URL + branch + optional PAT. Field ids are distinct from
// GitBuildSourceFields's own Repository URL / Branch inputs lower in the
// same form, so the two never collide as duplicate accessible names.
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
  )
}
