import {
  ArrowSquareOutIcon,
  BroomIcon,
  CheckCircleIcon,
  ClockIcon,
  GitPullRequestIcon,
  SpinnerIcon,
  TrashIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import type { VariantProps } from 'class-variance-authority'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { useGitSource } from '../queries/gitSources'
import {
  useSetPreviewEnabled,
  useSweepStalePreviewEnvironments,
  useTeardownPreviewEnvironment,
  usePreviewEnvironments,
} from '../queries/previewEnvironments'
import { ApiError } from '../lib/apiError'
import type { AppDetail } from '../types/appDetail'
import type { PreviewEnvironmentStatus } from '../types/previewEnvironment'

// Preview environments per pull request (internal/api/preview_environments.go):
// the opt-in toggle plus the active-preview list, rendered alongside
// GitSourceCard on the same overview route since a preview has nothing
// to build from without a connected git source. Off by default; opening
// a pull request against the connected repo's target branch deploys one
// automatically once this toggle is on, closing or merging it tears the
// preview down automatically too. "Tear down" here is the manual safety
// net for a stuck build or a retry after a partially-failed automatic
// teardown, not the primary path.

const STATUS_LABEL: Record<PreviewEnvironmentStatus, string> = {
  deploying: 'Deploying',
  active: 'Active',
  failed: 'Failed',
}

const STATUS_BADGE_VARIANT: Record<
  PreviewEnvironmentStatus,
  VariantProps<typeof badgeVariants>['variant']
> = {
  deploying: 'muted',
  active: 'success',
  failed: 'destructive',
}

const STATUS_ICON: Record<PreviewEnvironmentStatus, Icon> = {
  deploying: SpinnerIcon,
  active: CheckCircleIcon,
  failed: WarningCircleIcon,
}

function PreviewStatusBadge({ status }: { status: PreviewEnvironmentStatus }) {
  const StatusIcon = STATUS_ICON[status]
  return (
    <Badge variant={STATUS_BADGE_VARIANT[status]} className="rounded-full">
      <StatusIcon
        className={status === 'deploying' ? 'size-3 animate-spin' : 'size-3'}
        aria-hidden="true"
      />
      {STATUS_LABEL[status]}
    </Badge>
  )
}

export function PreviewEnvironmentsCard({ app }: { app: AppDetail }) {
  const gitSource = useGitSource(app.name)
  const setPreviewEnabled = useSetPreviewEnabled(app.name)
  const teardown = useTeardownPreviewEnvironment(app.name)
  const sweep = useSweepStalePreviewEnvironments()
  const previews = usePreviewEnvironments(app.name)

  const connected = !!gitSource.data
  const enabled = gitSource.data?.preview_enabled ?? false
  const hasStale = previews.data?.some((p) => p.stale) ?? false

  function toggle(next: boolean) {
    setPreviewEnabled.mutate(next, {
      onSuccess: () => {
        toast.add({
          title: next ? 'Preview environments enabled.' : 'Preview environments disabled.',
          type: 'success',
        })
      },
      onError: (error) => {
        toast.add({ title: 'Could not update preview environments.', description: error.message, type: 'error' })
      },
    })
  }

  function tearDown(prNumber: number) {
    teardown.mutate(prNumber, {
      onSuccess: () => {
        toast.add({ title: `Preview for PR #${prNumber} torn down.`, type: 'success' })
      },
      onError: (error: ApiError) => {
        toast.add({ title: 'Could not tear down preview.', description: error.message, type: 'error' })
      },
    })
  }

  function sweepStale() {
    sweep.mutate(undefined, {
      onSuccess: (result) => {
        toast.add({ title: `Swept ${result.swept} stale preview environment(s) across all apps.`, type: 'success' })
      },
      onError: (error: ApiError) => {
        toast.add({ title: 'Could not sweep stale previews.', description: error.message, type: 'error' })
      },
    })
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between gap-2">
          <CardTitle className="flex items-center gap-2">
            <GitPullRequestIcon className="size-4 text-muted-foreground" />
            Preview environments
          </CardTitle>
          {hasStale ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={sweep.isPending}
              onClick={sweepStale}
              title="Tears down every stale preview across all apps, not just this one."
            >
              <BroomIcon />
              Sweep stale previews
            </Button>
          ) : null}
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-center justify-between gap-4">
          <div>
            <p className="text-sm font-medium text-foreground">Enabled</p>
            <p className="text-sm text-muted-foreground">
              {connected
                ? 'Deploy an independent preview for every pull request opened against this app\'s repo, torn down automatically when the pull request closes.'
                : 'Connect a git source above first: a preview has nothing to build from otherwise.'}
            </p>
          </div>
          <Switch
            checked={enabled}
            onCheckedChange={toggle}
            disabled={!connected || setPreviewEnabled.isPending || gitSource.isLoading}
            aria-label="Preview environments enabled"
          />
        </div>

        {enabled ? (
          previews.isLoading ? (
            <p className="flex items-center gap-2 text-sm text-muted-foreground">
              <SpinnerIcon className="size-4 animate-spin" />
              Loading previews...
            </p>
          ) : previews.data && previews.data.length > 0 ? (
            <ul className="space-y-2">
              {previews.data.map((preview) => (
                <li
                  key={preview.pr_number}
                  className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-border p-2.5"
                >
                  <div className="min-w-0 space-y-1">
                    <div className="flex items-center gap-2">
                      <span className="font-mono text-sm">PR #{preview.pr_number}</span>
                      <PreviewStatusBadge status={preview.status} />
                      {preview.stale ? (
                        <Badge variant="warning" className="rounded-full">
                          <ClockIcon className="size-3" aria-hidden="true" />
                          Stale
                        </Badge>
                      ) : null}
                    </div>
                    <p className="truncate text-xs text-muted-foreground">
                      <span className="font-mono">{preview.branch}</span>
                      {preview.domain ? (
                        <>
                          {' · '}
                          <a
                            href={`https://${preview.domain}`}
                            target="_blank"
                            rel="noreferrer"
                            className="inline-flex items-center gap-1 font-mono text-foreground hover:underline"
                          >
                            {preview.domain}
                            <ArrowSquareOutIcon className="size-3" />
                          </a>
                        </>
                      ) : null}
                    </p>
                    {preview.status_reason ? (
                      <p className="text-xs text-amber-700 dark:text-amber-400">{preview.status_reason}</p>
                    ) : null}
                  </div>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    disabled={teardown.isPending}
                    onClick={() => { tearDown(preview.pr_number) }}
                  >
                    <TrashIcon />
                    Tear down
                  </Button>
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-sm text-muted-foreground">
              No active previews. Open a pull request against the connected repo to deploy one.
            </p>
          )
        ) : null}
      </CardContent>
    </Card>
  )
}
