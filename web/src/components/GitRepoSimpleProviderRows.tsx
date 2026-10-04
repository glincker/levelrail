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
import type {
  GitRepoSourceProvider,
  GitRepoSourceValue,
} from './GitRepoSourcePicker'

type RepoQuery<TRepo> = {
  data?: TRepo[]
  isLoading: boolean
  isError: boolean
  error: Error | null
}

type BranchQuery = {
  data?: { name: string }[]
  isLoading: boolean
  isError: boolean
  error: Error | null
}

type ProviderRef = NonNullable<GitRepoSourceValue['providerRef']>

// GitHub, Bitbucket and Gitea are the same shape (full_name-keyed
// repo-then-branch picker, and every *AppBranches hook is literally
// `(a: string, b: string, enabled: boolean) => BranchQuery`), so one
// generic component backs all three instead of three copy-pasted ones.
// A fifth provider of this same shape is one more `useRepos`/`fromRepo`/
// `useBranches`/`branchArgsFrom` config passed to it, not a new file.
function ConnectedProviderRow<TRepo>({
  provider,
  disabled,
  onSelect,
  runningRepoByUrl,
  providerKey,
  displayName,
  settingsPath,
  useRepos,
  fromRepo,
  useBranches,
  branchArgsFrom,
}: Readonly<
  ProviderRowProps & {
    providerKey: GitRepoSourceProvider
    displayName: string
    settingsPath: string
    useRepos: (enabled: boolean) => RepoQuery<TRepo>
    fromRepo: (repo: TRepo) => NormalizedRepoOption
    useBranches: (a: string, b: string, enabled: boolean) => BranchQuery
    branchArgsFrom: (ref: ProviderRef) => [string, string]
  }
>) {
  const enabled = provider.connected
  const repos = useRepos(enabled)
  const options = useMemo(
    () => (repos.data ?? []).map(fromRepo),
    [repos.data, fromRepo],
  )
  const [selectedRepoKey, setSelectedRepoKey] = useState('')
  const [selectedBranch, setSelectedBranch] = useState('')
  const selected = options.find((option) => option.key === selectedRepoKey)
  const [branchArgA, branchArgB] = selected
    ? branchArgsFrom(selected.providerRef)
    : ['', '']
  const branches = useBranches(branchArgA, branchArgB, selected !== undefined)
  const branchOptions = useMemo(
    () => (branches.data ?? []).map((b) => ({ value: b.name, label: b.name })),
    [branches.data],
  )

  if (!enabled) {
    return <NotConnectedPrompt name={displayName} settingsPath={settingsPath} />
  }

  const idPrefix = `git-picker-${providerKey}`

  return (
    <div className="space-y-2">
      <Field>
        <FieldLabel htmlFor={`${idPrefix}-repo`}>Repository</FieldLabel>
        <RepoPickerGrid
          searchInputId={`${idPrefix}-repo`}
          options={options}
          isLoading={repos.isLoading}
          isError={repos.isError}
          errorMessage={repos.error?.message}
          selectedKey={selectedRepoKey}
          disabled={disabled}
          runningRepoByUrl={runningRepoByUrl}
          onSelect={(option) => {
            setSelectedRepoKey(option.key)
            setSelectedBranch('')
          }}
        />
      </Field>

      {selected ? (
        <Field>
          <FieldLabel htmlFor={`${idPrefix}-branch`}>Branch</FieldLabel>
          <Combobox
            id={`${idPrefix}-branch`}
            options={branchOptions}
            value={selectedBranch}
            isLoading={branches.isLoading}
            disabled={disabled}
            placeholder="Select a branch"
            searchPlaceholder="Search branches..."
            onValueChange={(branch) => {
              setSelectedBranch(branch)
              onSelect({
                provider: providerKey,
                repoUrl: selected.cloneUrl,
                branch,
                providerRef: selected.providerRef,
              })
            }}
          />
          {branches.isError ? (
            <p className="text-sm text-destructive">
              {branches.error?.message}
            </p>
          ) : null}
        </Field>
      ) : null}
    </div>
  )
}

export function GitHubProviderRow(props: ProviderRowProps) {
  return (
    <ConnectedProviderRow
      {...props}
      providerKey="github"
      displayName="GitHub"
      settingsPath="/settings/github-app"
      useRepos={useGitHubAppRepos}
      fromRepo={fromGitHubRepo}
      useBranches={useGitHubAppBranches}
      branchArgsFrom={(ref) =>
        ref.kind === 'github' ? [ref.owner, ref.repo] : ['', '']
      }
    />
  )
}

export function BitbucketProviderRow(props: ProviderRowProps) {
  return (
    <ConnectedProviderRow
      {...props}
      providerKey="bitbucket"
      displayName="Bitbucket"
      settingsPath="/settings/bitbucket-app"
      useRepos={useBitbucketAppRepos}
      fromRepo={fromBitbucketRepo}
      useBranches={useBitbucketAppBranches}
      branchArgsFrom={(ref) =>
        ref.kind === 'bitbucket' ? [ref.workspace, ref.repoSlug] : ['', '']
      }
    />
  )
}

export function GiteaProviderRow(props: ProviderRowProps) {
  return (
    <ConnectedProviderRow
      {...props}
      providerKey="gitea"
      displayName="Gitea"
      settingsPath="/settings/gitea-app"
      useRepos={useGiteaAppRepos}
      fromRepo={fromGiteaRepo}
      useBranches={useGiteaAppBranches}
      branchArgsFrom={(ref) =>
        ref.kind === 'gitea' ? [ref.owner, ref.repo] : ['', '']
      }
    />
  )
}
