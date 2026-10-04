import { useMemo, useState } from 'react'
import {
  GithubLogoIcon,
  GitlabLogoIcon,
  GitBranchIcon,
  LinkIcon,
  TeaBagIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useGitProviders } from '../queries/gitProviders'
import { useGitSourceSummariesOptional } from '../queries/gitSources'
import { normalizeRepoUrl } from '../lib/gitRepoOptions'
import type { GitProviderStatus } from '../types/gitProviders'
import { GitLabProviderRow, ManualSourceRow } from './GitRepoSourceRows'
import {
  BitbucketProviderRow,
  GitHubProviderRow,
  GiteaProviderRow,
} from './GitRepoSimpleProviderRows'

// Shared "step 1" of CreateAppFromGitFields: a compact tab strip (one
// per connected provider, plus manual paste-URL) instead of all five
// stacked permanently, matching Vercel/Netlify/Railway's own import
// flows. Tab bodies live in GitRepoSourceRows.tsx /
// GitRepoSimpleProviderRows.tsx, split to stay under the 500-line cap.
export type GitRepoSourceProvider =
  'github' | 'gitlab' | 'bitbucket' | 'gitea' | 'manual'

// providerRef carries what each provider's "use as source" endpoint needs
// beyond a plain clone URL (owner+repo, project id, or workspace+slug).
// Manual picks have none, they use the generic PUT .../git-source call.
export interface GitRepoSourceValue {
  provider: GitRepoSourceProvider
  repoUrl: string
  branch: string
  /** Manual mode's optional deploy token for a private pasted repo. */
  token?: string
  providerRef?:
    | { kind: 'github'; owner: string; repo: string }
    | { kind: 'gitlab'; projectId: number }
    | { kind: 'bitbucket'; workspace: string; repoSlug: string }
    | { kind: 'gitea'; owner: string; repo: string }
}

// Defaults to "not connected" if the backend response omits an entry.
function findProvider(
  providers: GitProviderStatus[],
  name: GitProviderStatus['provider'],
): GitProviderStatus {
  return (
    providers.find((p) => p.provider === name) ?? {
      provider: name,
      connected: false,
      can_list_branches: false,
      can_register_webhook: false,
      can_auth_clone: false,
    }
  )
}

function ConnectedDot() {
  return (
    <span
      className="size-1.5 shrink-0 rounded-full bg-emerald-500"
      aria-hidden="true"
    />
  )
}

export function GitRepoSourcePicker({
  disabled,
  onSelect,
}: {
  disabled?: boolean
  /** Fires on every pick; the caller keeps whichever call came last. */
  onSelect: (value: GitRepoSourceValue) => void
}) {
  const [selected, setSelected] = useState<GitRepoSourceValue | null>(null)
  const { data: providers } = useGitProviders()
  const github = findProvider(providers, 'github')
  const gitlab = findProvider(providers, 'gitlab')
  const bitbucket = findProvider(providers, 'bitbucket')
  const gitea = findProvider(providers, 'gitea')

  // Computed once, shared by every tab body instead of re-fetching per tab.
  const gitSourceSummaries = useGitSourceSummariesOptional()
  const runningRepoByUrl = useMemo(() => {
    const map = new Map<string, string>()
    for (const summary of gitSourceSummaries.data ?? []) {
      map.set(normalizeRepoUrl(summary.repo_url), summary.service_name)
    }
    return map
  }, [gitSourceSummaries.data])

  const tabs: {
    key: GitRepoSourceProvider
    label: string
    icon: React.ReactNode
    connected: boolean
  }[] = [
    {
      key: 'github',
      label: 'GitHub',
      icon: <GithubLogoIcon className="size-4" aria-hidden="true" />,
      connected: github.connected,
    },
    {
      key: 'gitlab',
      label: 'GitLab',
      icon: <GitlabLogoIcon className="size-4" aria-hidden="true" />,
      connected: gitlab.connected,
    },
    {
      key: 'bitbucket',
      label: 'Bitbucket',
      icon: <GitBranchIcon className="size-4" aria-hidden="true" />,
      connected: bitbucket.connected,
    },
    {
      key: 'gitea',
      label: 'Gitea',
      icon: <TeaBagIcon className="size-4" aria-hidden="true" />,
      connected: gitea.connected,
    },
    {
      key: 'manual',
      label: 'URL',
      icon: <LinkIcon className="size-4" aria-hidden="true" />,
      connected: true,
    },
  ]

  // Auto-follows the connected provider until the operator picks a tab.
  const [manualTab, setManualTab] = useState<GitRepoSourceProvider | null>(null)
  const autoTab = tabs.find((t) => t.connected && t.key !== 'manual')?.key
  const activeTab = manualTab ?? autoTab ?? 'github'

  function handleSelect(value: GitRepoSourceValue) {
    setSelected(value)
    onSelect(value)
  }

  return (
    <div className="space-y-2">
      <Tabs
        value={activeTab}
        onValueChange={(value) => {
          if (typeof value === 'string') {
            setManualTab(value as GitRepoSourceProvider)
          }
        }}
      >
        <TabsList className="w-full">
          {tabs.map((tab) => (
            <TabsTrigger key={tab.key} value={tab.key} className="gap-1.5">
              {tab.icon}
              {tab.label}
              {tab.key !== 'manual' && tab.connected ? <ConnectedDot /> : null}
            </TabsTrigger>
          ))}
        </TabsList>

        <TabsContent value="github" className="pt-2">
          <GitHubProviderRow
            provider={github}
            disabled={disabled}
            onSelect={handleSelect}
            runningRepoByUrl={runningRepoByUrl}
          />
        </TabsContent>
        <TabsContent value="gitlab" className="pt-2">
          <GitLabProviderRow
            provider={gitlab}
            disabled={disabled}
            onSelect={handleSelect}
            runningRepoByUrl={runningRepoByUrl}
          />
        </TabsContent>
        <TabsContent value="bitbucket" className="pt-2">
          <BitbucketProviderRow
            provider={bitbucket}
            disabled={disabled}
            onSelect={handleSelect}
            runningRepoByUrl={runningRepoByUrl}
          />
        </TabsContent>
        <TabsContent value="gitea" className="pt-2">
          <GiteaProviderRow
            provider={gitea}
            disabled={disabled}
            onSelect={handleSelect}
            runningRepoByUrl={runningRepoByUrl}
          />
        </TabsContent>
        <TabsContent value="manual" className="pt-2">
          <ManualSourceRow disabled={disabled} onSelect={handleSelect} />
        </TabsContent>
      </Tabs>
      {selected ? (
        <p className="text-xs text-muted-foreground">
          Selected: <span className="font-mono">{selected.repoUrl}</span> @{' '}
          <span className="font-mono">{selected.branch}</span>
        </p>
      ) : null}
    </div>
  )
}
