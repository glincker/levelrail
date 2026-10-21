import { useMemo, useState } from 'react'
import { Field, FieldLabel } from '@/components/ui/field'
import { GitSelectedRepoBar } from './GitSelectedRepoBar'
import { GitConnectionBanner } from './GitConnectionHealth'
import { GitRepoListPicker } from './GitRepoListPicker'
import type { NormalizedRepoOption } from '../lib/gitRepoOptions'
import { useGitHubAppBranches } from '../queries/githubApp'
import { useGitConnectionBranches } from '../queries/gitConnections'
import type { GitRepoProvider } from '../types/gitRepos'
import { NotConnectedPrompt, type ProviderRowProps } from './GitRepoSourceRows'
import type {
  GitRepoSourceProvider,
  GitRepoSourceValue,
} from './GitRepoSourcePicker'

type BranchQuery = {
  data?: { name: string }[]
  isLoading: boolean
  isError: boolean
  error: Error | null
}

type ProviderRef = NonNullable<GitRepoSourceValue['providerRef']>

// GitHub, Bitbucket and Gitea are the same shape (a repo-then-branch picker,
// and every *AppBranches hook is `(a: string, b: string, enabled: boolean)`),
// so one component backs all three. A fourth provider of this shape is one
// more config passed to it, not a new file.
function ConnectedProviderRow({
  provider,
  disabled,
  onSelect,
  runningRepoByUrl,
  providerKey,
  displayName,
  settingsPath,
  useBranches,
  branchArgsFrom,
}: Readonly<
  ProviderRowProps & {
    providerKey: GitRepoSourceProvider & GitRepoProvider
    displayName: string
    settingsPath: string
    useBranches: (a: string, b: string, enabled: boolean) => BranchQuery
    branchArgsFrom: (ref: ProviderRef) => [string, string]
  }
>) {
  const enabled = provider.connected
  const [selected, setSelected] = useState<NormalizedRepoOption | null>(null)
  const [selectedBranch, setSelectedBranch] = useState('')
  const [changing, setChanging] = useState(false)
  const [branchArgA, branchArgB] = selected
    ? branchArgsFrom(selected.providerRef)
    : ['', '']
  const branches = useBranches(branchArgA, branchArgB, selected !== null)
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
      <GitConnectionBanner provider={provider} />
      {selected && !changing ? (
        <GitSelectedRepoBar
          option={selected}
          branch={selectedBranch}
          branchOptions={branchOptions}
          branchesLoading={branches.isLoading}
          branchError={branches.isError ? branches.error?.message : undefined}
          disabled={disabled}
          idPrefix={idPrefix}
          onBranchChange={(branch) => {
            setSelectedBranch(branch)
            onSelect({
              provider: providerKey,
              repoUrl: selected.cloneUrl,
              branch,
              providerRef: selected.providerRef,
            })
          }}
          onChange={() => {
            setChanging(true)
          }}
        />
      ) : (
        <Field>
          <FieldLabel htmlFor={`${idPrefix}-repo`}>Repository</FieldLabel>
          <GitRepoListPicker
            provider={providerKey}
            enabled={enabled}
            searchInputId={`${idPrefix}-repo`}
            selectedKey={selected?.key ?? ''}
            disabled={disabled}
            runningRepoByUrl={runningRepoByUrl}
            onSelect={(option) => {
              setSelected(option)
              setSelectedBranch(option.defaultBranch)
              setChanging(false)
              onSelect({
                provider: providerKey,
                repoUrl: option.cloneUrl,
                branch: option.defaultBranch,
                providerRef: option.providerRef,
              })
            }}
          />
        </Field>
      )}
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
      useBranches={useGitConnectionBranches}
      branchArgsFrom={(ref) =>
        ref.kind === 'bitbucket' ? [ref.connectionId, ref.repo] : ['', '']
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
      useBranches={useGitConnectionBranches}
      branchArgsFrom={(ref) =>
        ref.kind === 'gitea' ? [ref.connectionId, ref.repo] : ['', '']
      }
    />
  )
}
