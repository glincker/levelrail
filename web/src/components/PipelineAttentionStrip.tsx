import { Link } from '@tanstack/react-router'
import {
  ThumbsDownIcon,
  ThumbsUpIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import {
  useAttentionRuns,
  useDecidePipelineRow,
} from '../queries/pipelineOverview'
import {
  attentionRows,
  decisionLabel,
  isPendingDecision,
} from '../lib/pipelineOverview'
import type { PipelineRunRow } from '../types/pipelineOverview'
import { PipelineStatusIcon } from './PipelineStatusBadge'

const PENDING_LIMIT = 10
const FAILED_LIMIT = 5

function AttentionItem({ row }: { row: PipelineRunRow }) {
  const decide = useDecidePipelineRow()
  const pending = isPendingDecision(row)
  const act = (decision: 'approved' | 'rejected') =>
    decide.mutate(
      { row, decision },
      { onError: (e) => toast.add({ title: e.message, type: 'error' }) },
    )
  return (
    <li className="flex flex-wrap items-center gap-2 py-2">
      <PipelineStatusIcon
        status={row.status}
        className={
          pending
            ? 'size-4 text-amber-600 dark:text-amber-400'
            : 'size-4 text-destructive'
        }
      />
      <Link
        to="/apps/$name/pipelines/runs/$runId"
        params={{ name: row.app, runId: row.id }}
        className="min-w-40 flex-1 text-sm text-foreground underline-offset-2 hover:underline"
      >
        <span className="font-medium">
          {row.app} / {row.pipeline} #{row.number}
        </span>
        <span className="ml-2 text-xs text-muted-foreground">
          {pending ? decisionLabel(row) : (row.reason ?? 'Failed')}
        </span>
      </Link>
      {pending && row.can_decide ? (
        <>
          <Button
            size="sm"
            disabled={decide.isPending}
            onClick={() => act('approved')}
          >
            <ThumbsUpIcon aria-hidden="true" />
            Approve
          </Button>
          <Button
            size="sm"
            variant="destructive"
            disabled={decide.isPending}
            onClick={() => act('rejected')}
          >
            <ThumbsDownIcon aria-hidden="true" />
            Reject
          </Button>
        </>
      ) : null}
    </li>
  )
}

export function PipelineAttentionStrip() {
  const waiting = useAttentionRuns(
    { status: 'waiting_approval' },
    PENDING_LIMIT,
  )
  const held = useAttentionRuns({ status: 'held' }, PENDING_LIMIT)
  const failed = useAttentionRuns({ status: 'failed' }, FAILED_LIMIT)
  const rows = attentionRows(
    [...(waiting.data?.runs ?? []), ...(held.data?.runs ?? [])],
    failed.data?.runs ?? [],
    failed.dataUpdatedAt,
  )
  if (rows.length === 0) {
    return null
  }
  return (
    <section
      aria-label="Needs attention"
      className="rounded-lg border border-amber-300 bg-amber-50 p-3 dark:border-amber-800 dark:bg-amber-950/30"
    >
      <h2 className="flex items-center gap-2 text-sm font-semibold text-foreground">
        <WarningCircleIcon className="size-4" aria-hidden="true" />
        Needs attention
      </h2>
      <ul className="mt-1 divide-y divide-amber-200 dark:divide-amber-900">
        {rows.map((row) => (
          <AttentionItem key={row.id} row={row} />
        ))}
      </ul>
    </section>
  )
}
