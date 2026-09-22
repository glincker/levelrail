import { useState } from 'react'
import { useGitProviders } from '../queries/gitProviders'
import type { GitProviderStatus } from '../types/gitProviders'
import {
  BitbucketProviderRow,
  GitHubProviderRow,
  GiteaProviderRow,
  GitLabProviderRow,
  ManualSourceRow,
} from './GitRepoSourceRows'

// GitRepoSourcePicker is the shared "step 1" of both CreateAppFromGitFields
// and a future GitSourceCard: one list, one mental model, for all four
// connected git providers plus a manual paste-a-URL fallback, instead of a
// different widget per entry point. Fixes two bugs: the GitHub wizard path
// never created a git_source row (no continuous deployment), and the
// GitLab/Bitbucket settings-page paths never triggered a first build
// (nothing running until a manual push).
//
// Connection status and per-provider capability (branch listing, webhook
// registration) come from one aggregated GET /api/v1/git-providers call
// (useGitProviders) rather than three separate per-provider status
// queries: see that hook's own doc comment for why. Each row reads its
// own can_list_branches flag to decide whether to render a branch Select
// or a free-text fallback, rather than hardcoding "GitLab doesn't have
// this": proposal section 3's own closing point, now that piece 1 (GitLab
// ListBranches) has closed that gap, is that the flag stays the source of
// truth going forward instead of a per-provider assumption baked into the
// component.
//
// The four provider rows plus the manual fallback live in
// GitRepoSourceRows.tsx, split out to keep both files under the 500-line
// cap; this file is purely the orchestrator.
export type GitRepoSourceProvider =
  'github' | 'gitlab' | 'bitbucket' | 'gitea' | 'manual'

// providerRef carries what each provider's "use as source" endpoint needs
// beyond a plain clone URL: GitHub's owner+repo pair
// (connectGitHubRepoAsSource, queries/githubApp.ts), GitLab's numeric
// project id (connectGitLabProjectAsSource, queries/gitlabApp.ts),
// Bitbucket's workspace+repo slug pair (connectBitbucketRepoAsSource,
// queries/bitbucketApp.ts), or Gitea's owner+repo pair
// (connectGiteaRepoAsSource, queries/giteaApp.ts). Manual picks have no
// such endpoint: they degrade to the generic PUT .../git-source call,
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
    | { kind: 'github'; owner: string; repo: string }
    | { kind: 'gitlab'; projectId: number }
    | { kind: 'bitbucket'; workspace: string; repoSlug: string }
    | { kind: 'gitea'; owner: string; repo: string }
}

// findProvider looks up one provider's aggregated status by name,
// defaulting to "not connected, no capabilities" if the backend response
// somehow omits it: every row already guards its "connected" UI on this
// value, so a missing entry degrades to the same not-connected empty
// state a real disconnected provider gets, never a crash.
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

// GitRepoSourcePicker itself: four provider rows plus the manual
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
  const { data: providers } = useGitProviders()
  const github = findProvider(providers, 'github')
  const gitlab = findProvider(providers, 'gitlab')
  const bitbucket = findProvider(providers, 'bitbucket')
  const gitea = findProvider(providers, 'gitea')

  function handleSelect(value: GitRepoSourceValue) {
    setSelected(value)
    onSelect(value)
  }

  return (
    <div className="space-y-3">
      <div className="space-y-4 rounded-lg border border-dashed border-border p-3">
        <GitHubProviderRow
          provider={github}
          disabled={disabled}
          onSelect={handleSelect}
        />
        <div className="border-t border-border" />
        <GitLabProviderRow
          provider={gitlab}
          disabled={disabled}
          onSelect={handleSelect}
        />
        <div className="border-t border-border" />
        <BitbucketProviderRow
          provider={bitbucket}
          disabled={disabled}
          onSelect={handleSelect}
        />
        <div className="border-t border-border" />
        <GiteaProviderRow
          provider={gitea}
          disabled={disabled}
          onSelect={handleSelect}
        />
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
