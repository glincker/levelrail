import { Link } from '@tanstack/react-router'
import {
  CircleAlertGlassIcon,
  ThumbsDownGlassIcon,
  ThumbsUpGlassIcon,
} from '@thesvg/ui-react'
import { useTranslation } from 'react-i18next'
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
import { Alert } from '@/components/ui/alert'

const PENDING_LIMIT = 10
const FAILED_LIMIT = 5

function AttentionItem({ row }: { row: PipelineRunRow }) {
  const { t } = useTranslation('pipelines')
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
          pending ? 'size-5 text-tone-warning' : 'size-5 text-destructive'
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
          {pending ? decisionLabel(row) : (row.reason ?? t('attention.failed'))}
        </span>
      </Link>
      {pending && row.can_decide ? (
        <>
          <Button disabled={decide.isPending} onClick={() => act('approved')}>
            <ThumbsUpGlassIcon size="1.4em" aria-hidden="true" />
            {t('attention.approve')}
          </Button>
          <Button
            variant="destructive"
            disabled={decide.isPending}
            onClick={() => act('rejected')}
          >
            <ThumbsDownGlassIcon size="1.4em" aria-hidden="true" />
            {t('attention.reject')}
          </Button>
        </>
      ) : null}
    </li>
  )
}

export function PipelineAttentionStrip() {
  const { t } = useTranslation('pipelines')
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
    <Alert
      variant="warning"
      role="region"
      aria-label={t('attention.title')}
      className="block p-3"
    >
      <h2 className="flex items-center gap-2 text-sm font-semibold text-foreground">
        <CircleAlertGlassIcon
          className="size-5 text-tone-danger"
          aria-hidden="true"
        />
        {t('attention.title')}
      </h2>
      <ul className="mt-1 divide-y divide-tone-warning-border">
        {rows.map((row) => (
          <AttentionItem key={row.id} row={row} />
        ))}
      </ul>
    </Alert>
  )
}
