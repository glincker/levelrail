import { useMemo, useState } from 'react'
import { Combobox } from '@/components/ui/combobox'
import { Field, FieldLabel } from '@/components/ui/field'
import { cn } from '@/lib/utils'
import { GroupedRepoPickerGrid, RepoPickerGrid } from './GitRepoPickerGrid'
import {
  fromBitbucketRepo,
  fromGitHubRepo,
  fromGiteaRepo,
  groupGitHubRepos,
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

// AccountFilterChip is one entry of GitHubProviderRow's filter strip:
// "All" plus one per connected account/org. Plain buttons, not the
// Tabs primitive GitRepoSourcePicker uses one level up: these filter
// client-side over an already-fetched list, they don't switch panels.
function AccountFilterChip({
  label,
  active,
  hasError,
  onClick,
}: Readonly<{
  label: string
  active: boolean
  hasError?: boolean
  onClick: () => void
}>) {
  return (
    <button
      type="button"
      aria-pressed={active}
      onClick={onClick}
      className={cn(
        'rounded-full border px-2.5 py-1 text-xs font-medium transition-colors',
        active
          ? 'border-primary/50 bg-primary/10 text-primary'
          : 'border-border text-muted-foreground hover:bg-muted',
        hasError &&
          !active &&
          'border-amber-500/40 text-amber-700 dark:text-amber-400',
      )}
    >
      {label}
    </button>
  )
}

// GitHub is the one provider whose App can be installed on more than one
// account/org (migrations/0282), so it gets its own row instead of
// ConnectedProviderRow's single-fetch shape: the repo fetch itself
// returns {repos, errors} grouped by account, and a filter strip lets an
// operator narrow the grid to one account without a re-fetch.
export function GitHubProviderRow({
  provider,
  disabled,
  onSelect,
  runningRepoByUrl,
}: ProviderRowProps) {
  const enabled = provider.connected
  const repos = useGitHubAppRepos(enabled)
  const [selectedRepoKey, setSelectedRepoKey] = useState('')
  const [selectedBranch, setSelectedBranch] = useState('')
  const [accountFilter, setAccountFilter] = useState('all')

  const allOptions = useMemo(
    () => (repos.data?.repos ?? []).map(fromGitHubRepo),
    [repos.data],
  )
  const groups = useMemo(
    () => groupGitHubRepos(allOptions, repos.data?.errors ?? []),
    [allOptions, repos.data],
  )
  const visibleGroups = useMemo(
    () =>
      accountFilter === 'all'
        ? groups
        : groups.filter((g) => g.key === accountFilter),
    [groups, accountFilter],
  )

  const selected = allOptions.find((option) => option.key === selectedRepoKey)
  const providerRef = selected?.providerRef
  const [branchOwner, branchRepo] =
    providerRef?.kind === 'github'
      ? [providerRef.owner, providerRef.repo]
      : ['', '']
  const branches = useGitHubAppBranches(
    branchOwner,
    branchRepo,
    selected !== undefined,
  )
  const branchOptions = useMemo(
    () => (branches.data ?? []).map((b) => ({ value: b.name, label: b.name })),
    [branches.data],
  )

  if (!enabled) {
    return (
      <NotConnectedPrompt name="GitHub" settingsPath="/settings/github-app" />
    )
  }

  return (
    <div className="space-y-2">
      <Field>
        <FieldLabel htmlFor="git-picker-github-repo">Repository</FieldLabel>
        {groups.length > 1 ? (
          <div className="flex flex-wrap gap-1.5">
            <AccountFilterChip
              label="All"
              active={accountFilter === 'all'}
              onClick={() => setAccountFilter('all')}
            />
            {groups.map((group) => (
              <AccountFilterChip
                key={group.key || 'unknown'}
                label={group.label}
                active={accountFilter === group.key}
                hasError={Boolean(group.error)}
                onClick={() => setAccountFilter(group.key)}
              />
            ))}
          </div>
        ) : null}
        <GroupedRepoPickerGrid
          searchInputId="git-picker-github-repo"
          groups={visibleGroups}
          showHeaders={groups.length > 1}
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
          <FieldLabel htmlFor="git-picker-github-branch">Branch</FieldLabel>
          <Combobox
            id="git-picker-github-branch"
            options={branchOptions}
            value={selectedBranch}
            isLoading={branches.isLoading}
            disabled={disabled}
            placeholder="Select a branch"
            searchPlaceholder="Search branches..."
            onValueChange={(branch) => {
              setSelectedBranch(branch)
              onSelect({
                provider: 'github',
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
