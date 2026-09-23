import { useState } from 'react'
import type { ComponentType } from 'react'
import {
  ArrowSquareOutIcon,
  CheckCircleIcon,
  GitBranchIcon,
  GithubLogoIcon,
  GitlabLogoIcon,
  TeaBagIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ListSkeleton } from '@/components/ui/list-skeleton'
import type { GitProviderName } from '../../types/gitProviders'
import { gitGate } from '../../lib/setupWizard'
import { StepFooter } from './StepChrome'
import { useGitProviderPolling } from './useSetupPolling'
import type { StepProps } from './types'

const PROVIDERS: Record<
  GitProviderName,
  { label: string; href: string; icon: ComponentType<{ className?: string }> }
> = {
  github: {
    label: 'GitHub',
    href: '/settings/github-app',
    icon: GithubLogoIcon,
  },
  gitlab: {
    label: 'GitLab',
    href: '/settings/gitlab-app',
    icon: GitlabLogoIcon,
  },
  bitbucket: {
    label: 'Bitbucket',
    href: '/settings/bitbucket-app',
    icon: GitBranchIcon,
  },
  gitea: { label: 'Gitea', href: '/settings/gitea-app', icon: TeaBagIcon },
}

/** GitProviderStep links to each provider's connect flow and notices when one connects. */
export function GitProviderStep({ onContinue, onSkip, pending }: StepProps) {
  const [startedAt] = useState(() => Date.now())
  const { data: providers, error } = useGitProviderPolling(startedAt)
  const gate = providers ? gitGate(providers) : gitGate([])

  return (
    <div className="space-y-4">
      <p className="max-w-prose text-sm text-muted-foreground">
        Optional. Connect a git provider to deploy private repositories and
        redeploy on every push. Each one opens in a new tab; this page notices
        as soon as the connection finishes.
      </p>

      {error ? (
        <p className="text-xs text-destructive">{error.message}</p>
      ) : null}

      {providers ? (
        <ul className="grid gap-2 sm:grid-cols-2">
          {providers.map((p) => {
            const meta = PROVIDERS[p.provider]
            const ProviderIcon = meta.icon
            return (
              <li
                key={p.provider}
                className="flex items-center justify-between gap-3 rounded-lg border border-border p-3"
              >
                <span className="flex items-center gap-2 text-sm font-medium text-foreground">
                  <ProviderIcon className="size-5 text-muted-foreground" />
                  {meta.label}
                </span>
                {p.connected ? (
                  <Badge variant="success">
                    <CheckCircleIcon />
                    Connected
                  </Badge>
                ) : (
                  <Button
                    size="sm"
                    variant="outline"
                    render={
                      <a href={meta.href} target="_blank" rel="noreferrer" />
                    }
                  >
                    Connect
                    <ArrowSquareOutIcon />
                  </Button>
                )}
              </li>
            )
          })}
        </ul>
      ) : (
        <ListSkeleton rows={2} />
      )}

      <StepFooter
        gate={gate}
        onContinue={onContinue}
        onSkip={onSkip}
        pending={pending}
      />
    </div>
  )
}
