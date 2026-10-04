import { useMemo, useState } from 'react'
import { Combobox } from '@/components/ui/combobox'
import { Field, FieldLabel } from '@/components/ui/field'
import { RepoPickerGrid } from './GitRepoPickerGrid'
import {
  fromBitbucketRepo,
  fromGitHubRepo,
  fromGiteaRepo,
  type NormalizedRepoOption,
} from '../lib/gitRepoOptions'
import { useGitHubAppBranches, useGitHubAppRepos } from '../queries/githubApp'
import {
  useBitbucketAppBranches,
  useBitbucketAppRepos,
} from '../queries/bitbucketApp'
import { useGiteaAppBranches, useGiteaAppRepos } from '../queries/giteaApp'
import { NotConnectedPrompt, type ProviderRowProps } from './GitRepoSourceRows'
import type { GitRepoSourceValue } from './GitRepoSourcePicker'

// GitHub/Bitbucket/Gitea tabs: structurally identical (full_name-keyed
// repo-then-branch picker), split from GitRepoSourceRows.tsx for the
// 500-line cap. SimpleProviderRepoBody is controlled: each
// *RepoBranchFields wrapper owns selectedRepoKey/selectedBranch itself
// since its branches hook needs owner/name split out of that same state.
function SimpleProviderRepoBody({
  idPrefix,
  options,
  repoState,
  branchState,
  selectedRepoKey,
  selectedBranch,
  disabled,
  runningRepoByUrl,
  onRepoSelect,
  onBranchSelect,
}: {
  idPrefix: string
  options: NormalizedRepoOption[]
  repoState: { isLoading: boolean; isError: boolean; error: Error | null }
  branchState: {
    data?: { name: string }[]
    isLoading: boolean
    isError: boolean
    error: Error | null
  }
  selectedRepoKey: string
  selectedBranch: string
  disabled?: boolean
  runningRepoByUrl: Map<string, string>
  onRepoSelect: (option: NormalizedRepoOption) => void
  onBranchSelect: (branch: string) => void
}) {
  const selected = options.find((option) => option.key === selectedRepoKey)
  const branchOptions = useMemo(
    () =>
      (branchState.data ?? []).map((b) => ({ value: b.name, label: b.name })),
    [branchState.data],
  )

  return (
    <div className="space-y-2">
      <Field>
        <FieldLabel htmlFor={`${idPrefix}-repo`}>Repository</FieldLabel>
        <RepoPickerGrid
          searchInputId={`${idPrefix}-repo`}
          options={options}
          isLoading={repoState.isLoading}
          isError={repoState.isError}
          errorMessage={repoState.error?.message}
          selectedKey={selectedRepoKey}
          disabled={disabled}
          runningRepoByUrl={runningRepoByUrl}
          onSelect={onRepoSelect}
        />
      </Field>

      {selected ? (
        <Field>
          <FieldLabel htmlFor={`${idPrefix}-branch`}>Branch</FieldLabel>
          <Combobox
            id={`${idPrefix}-branch`}
            options={branchOptions}
            value={selectedBranch}
            isLoading={branchState.isLoading}
            disabled={disabled}
            placeholder="Select a branch"
            searchPlaceholder="Search branches..."
            onValueChange={onBranchSelect}
          />
          {branchState.isError ? (
            <p className="text-sm text-destructive">
              {branchState.error?.message}
            </p>
          ) : null}
        </Field>
      ) : null}
    </div>
  )
}

export function GitHubProviderRow({
  provider,
  disabled,
  onSelect,
  runningRepoByUrl,
}: ProviderRowProps) {
  const enabled = provider.connected
  const repos = useGitHubAppRepos(enabled)
  const options = useMemo(
    () => (repos.data ?? []).map(fromGitHubRepo),
    [repos.data],
  )

  if (!enabled) {
    return (
      <NotConnectedPrompt name="GitHub" settingsPath="/settings/github-app" />
    )
  }

  return (
    <GitHubRepoBranchFields
      options={options}
      repos={repos}
      disabled={disabled}
      onSelect={onSelect}
      runningRepoByUrl={runningRepoByUrl}
    />
  )
}

// Split from GitHubProviderRow purely so useGitHubAppBranches (which
// needs the selected repo's owner/name split out of its key) can live
// next to the selectedRepoKey state it depends on.
function GitHubRepoBranchFields({
  options,
  repos,
  disabled,
  onSelect,
  runningRepoByUrl,
}: {
  options: NormalizedRepoOption[]
  repos: { isLoading: boolean; isError: boolean; error: Error | null }
  disabled?: boolean
  onSelect: (value: GitRepoSourceValue) => void
  runningRepoByUrl: Map<string, string>
}) {
  const [selectedRepoKey, setSelectedRepoKey] = useState('')
  const [selectedBranch, setSelectedBranch] = useState('')
  const selected = options.find((option) => option.key === selectedRepoKey)
  const [owner, repoName] =
    selected?.providerRef.kind === 'github'
      ? [selected.providerRef.owner, selected.providerRef.repo]
      : ['', '']
  const branches = useGitHubAppBranches(owner, repoName, selected !== undefined)

  return (
    <SimpleProviderRepoBody
      idPrefix="git-picker-github"
      options={options}
      repoState={repos}
      branchState={branches}
      selectedRepoKey={selectedRepoKey}
      selectedBranch={selectedBranch}
      disabled={disabled}
      runningRepoByUrl={runningRepoByUrl}
      onRepoSelect={(option) => {
        setSelectedRepoKey(option.key)
        setSelectedBranch('')
      }}
      onBranchSelect={(branch) => {
        setSelectedBranch(branch)
        if (!selected) return
        onSelect({
          provider: 'github',
          repoUrl: selected.cloneUrl,
          branch,
          providerRef: selected.providerRef,
        })
      }}
    />
  )
}

export function BitbucketProviderRow({
  provider,
  disabled,
  onSelect,
  runningRepoByUrl,
}: ProviderRowProps) {
  const enabled = provider.connected
  const repos = useBitbucketAppRepos(enabled)
  const options = useMemo(
    () => (repos.data ?? []).map(fromBitbucketRepo),
    [repos.data],
  )

  if (!enabled) {
    return (
      <NotConnectedPrompt
        name="Bitbucket"
        settingsPath="/settings/bitbucket-app"
      />
    )
  }

  return (
    <BitbucketRepoBranchFields
      options={options}
      repos={repos}
      disabled={disabled}
      onSelect={onSelect}
      runningRepoByUrl={runningRepoByUrl}
    />
  )
}

function BitbucketRepoBranchFields({
  options,
  repos,
  disabled,
  onSelect,
  runningRepoByUrl,
}: {
  options: NormalizedRepoOption[]
  repos: { isLoading: boolean; isError: boolean; error: Error | null }
  disabled?: boolean
  onSelect: (value: GitRepoSourceValue) => void
  runningRepoByUrl: Map<string, string>
}) {
  const [selectedRepoKey, setSelectedRepoKey] = useState('')
  const [selectedBranch, setSelectedBranch] = useState('')
  const selected = options.find((option) => option.key === selectedRepoKey)
  const [workspace, repoSlug] =
    selected?.providerRef.kind === 'bitbucket'
      ? [selected.providerRef.workspace, selected.providerRef.repoSlug]
      : ['', '']
  const branches = useBitbucketAppBranches(
    workspace,
    repoSlug,
    selected !== undefined,
  )

  return (
    <SimpleProviderRepoBody
      idPrefix="git-picker-bitbucket"
      options={options}
      repoState={repos}
      branchState={branches}
      selectedRepoKey={selectedRepoKey}
      selectedBranch={selectedBranch}
      disabled={disabled}
      runningRepoByUrl={runningRepoByUrl}
      onRepoSelect={(option) => {
        setSelectedRepoKey(option.key)
        setSelectedBranch('')
      }}
      onBranchSelect={(branch) => {
        setSelectedBranch(branch)
        if (!selected) return
        onSelect({
          provider: 'bitbucket',
          repoUrl: selected.cloneUrl,
          branch,
          providerRef: selected.providerRef,
        })
      }}
    />
  )
}

export function GiteaProviderRow({
  provider,
  disabled,
  onSelect,
  runningRepoByUrl,
}: ProviderRowProps) {
  const enabled = provider.connected
  const repos = useGiteaAppRepos(enabled)
  const options = useMemo(
    () => (repos.data ?? []).map(fromGiteaRepo),
    [repos.data],
  )

  if (!enabled) {
    return (
      <NotConnectedPrompt name="Gitea" settingsPath="/settings/gitea-app" />
    )
  }

  return (
    <GiteaRepoBranchFields
      options={options}
      repos={repos}
      disabled={disabled}
      onSelect={onSelect}
      runningRepoByUrl={runningRepoByUrl}
    />
  )
}

function GiteaRepoBranchFields({
  options,
  repos,
  disabled,
  onSelect,
  runningRepoByUrl,
}: {
  options: NormalizedRepoOption[]
  repos: { isLoading: boolean; isError: boolean; error: Error | null }
  disabled?: boolean
  onSelect: (value: GitRepoSourceValue) => void
  runningRepoByUrl: Map<string, string>
}) {
  const [selectedRepoKey, setSelectedRepoKey] = useState('')
  const [selectedBranch, setSelectedBranch] = useState('')
  const selected = options.find((option) => option.key === selectedRepoKey)
  const [owner, repoName] =
    selected?.providerRef.kind === 'gitea'
      ? [selected.providerRef.owner, selected.providerRef.repo]
      : ['', '']
  const branches = useGiteaAppBranches(owner, repoName, selected !== undefined)

  return (
    <SimpleProviderRepoBody
      idPrefix="git-picker-gitea"
      options={options}
      repoState={repos}
      branchState={branches}
      selectedRepoKey={selectedRepoKey}
      selectedBranch={selectedBranch}
      disabled={disabled}
      runningRepoByUrl={runningRepoByUrl}
      onRepoSelect={(option) => {
        setSelectedRepoKey(option.key)
        setSelectedBranch('')
      }}
      onBranchSelect={(branch) => {
        setSelectedBranch(branch)
        if (!selected) return
        onSelect({
          provider: 'gitea',
          repoUrl: selected.cloneUrl,
          branch,
          providerRef: selected.providerRef,
        })
      }}
    />
  )
}
