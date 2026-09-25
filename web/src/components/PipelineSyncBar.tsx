import {
  ArrowsClockwiseIcon,
  GitBranchIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import {
  usePipelineSync,
  useRunPipelineSync,
  useSetPipelineRepoTruth,
} from '../queries/pipelines'
import type { PipelineSyncOutcome } from '../types/pipelines'
import type { BadgeVariant } from '../lib/pipelineStatus'

const OUTCOME_VARIANT: Record<PipelineSyncOutcome, BadgeVariant> = {
  created: 'success',
  updated: 'success',
  unchanged: 'muted',
  diverged: 'warning',
  invalid: 'destructive',
  refused: 'destructive',
}

// Where pipeline definitions come from: the repository's pipeline directory,
// synced on every push to the tracked branch or on demand here.
export function PipelineSyncBar({ appName }: { appName: string }) {
  const { data: status } = usePipelineSync(appName)
  const sync = useRunPipelineSync(appName)
  const setTruth = useSetPipelineRepoTruth(appName)
  if (!status) {
    return null
  }
  if (!status.connected) {
    return (
      <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
        <GitBranchIcon className="size-4" aria-hidden="true" />
        Connect a git repository to keep pipelines in sync with your pipeline
        directory.
      </p>
    )
  }
  return (
    <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-3">
      <div className="flex flex-wrap items-center gap-3">
        <div className="flex min-w-48 flex-1 flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <GitBranchIcon className="size-4" aria-hidden="true" />
          {status.last_sha ? (
            <Badge variant="outline" title={status.last_sha}>
              synced from {status.last_sha.slice(0, 7)}
            </Badge>
          ) : (
            <span>Not synced yet</span>
          )}
          {status.last_synced_at ? (
            <span>{new Date(status.last_synced_at).toLocaleString()}</span>
          ) : null}
        </div>
        <div className="flex items-center gap-2 text-xs text-foreground">
          <Switch
            checked={status.repo_is_truth}
            disabled={setTruth.isPending}
            aria-label="Repository is source of truth"
            onCheckedChange={(next) =>
              setTruth.mutate(next, {
                onError: (e) => toast.add({ title: e.message, type: 'error' }),
              })
            }
          />
          Repository is source of truth
        </div>
        <Button
          variant="outline"
          size="sm"
          disabled={sync.isPending}
          onClick={() =>
            sync.mutate(undefined, {
              onError: (e) => toast.add({ title: e.message, type: 'error' }),
            })
          }
        >
          <ArrowsClockwiseIcon aria-hidden="true" />
          {sync.isPending ? 'Syncing...' : 'Sync now'}
        </Button>
      </div>
      {status.last_error ? (
        <p className="text-xs text-destructive">{status.last_error}</p>
      ) : null}
      {sync.data ? (
        <ul className="space-y-1 text-xs" aria-label="Sync result">
          {sync.data.items.length === 0 ? (
            <li className="text-muted-foreground">
              No pipeline files found in the repository.
            </li>
          ) : (
            sync.data.items.map((it) => (
              <li key={it.file} className="flex flex-wrap items-center gap-2">
                <Badge variant={OUTCOME_VARIANT[it.outcome]}>
                  {it.outcome}
                </Badge>
                <span className="font-medium text-foreground">{it.file}</span>
                {it.message ? (
                  <span className="text-muted-foreground">{it.message}</span>
                ) : null}
              </li>
            ))
          )}
        </ul>
      ) : null}
    </div>
  )
}
