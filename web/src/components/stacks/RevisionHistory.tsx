import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowCounterClockwiseIcon,
  CheckIcon,
  EyeIcon,
  GitDiffIcon,
  PushPinIcon,
  PushPinSlashIcon,
  XIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { RelativeTime } from '@/components/kit'
import type { Stack, StackRevisionSummary } from '../../types/stacks'
import { canApprove, canRollbackTo } from './approval'
import { RevisionStatus } from './StackIndicators'

export interface RevisionHistoryProps {
  stack: Stack
  isRoot: boolean
  currentUser: string | null
  revisions: readonly StackRevisionSummary[]
  selected?: number
  onView: (n: number) => void
  onDiff: (n: number) => void
  onRollback: (n: number) => void
  onPin: (n: number | null) => void
  onApprove: (rev: StackRevisionSummary) => void
  onReject: (n: number) => void
}

export function RevisionHistory({
  stack,
  isRoot,
  currentUser,
  revisions,
  selected,
  onView,
  onDiff,
  onRollback,
  onPin,
  onApprove,
  onReject,
}: Readonly<RevisionHistoryProps>) {
  const { t } = useTranslation('stacks')
  const columns = useMemo<ResponsiveColumn<StackRevisionSummary>[]>(
    () => [
      {
        id: 'number',
        header: t('history.columns.number'),
        primary: true,
        cell: (r) => (
          <span className="flex flex-wrap items-center gap-1.5">
            <span className="font-mono text-xs font-medium">
              {t('detail.revisionShort', { number: r.number })}
            </span>
            {stack.pinned_revision === r.number ? (
              <Badge variant="info">{t('history.pinnedMark')}</Badge>
            ) : null}
            {r.rollback_of ? (
              <Badge variant="muted">
                {t('history.rollbackOf', { number: r.rollback_of })}
              </Badge>
            ) : null}
          </span>
        ),
      },
      {
        id: 'status',
        header: t('history.columns.status'),
        cell: (r) => <RevisionStatus revision={r} />,
      },
      {
        id: 'actor',
        header: t('history.columns.actor'),
        priority: 2,
        cell: (r) => <span className="text-sm">{r.actor}</span>,
      },
      {
        id: 'source',
        header: t('history.columns.source'),
        priority: 3,
        cell: (r) => (
          <span className="font-mono text-xs text-muted-foreground">
            {r.git_sha ? r.git_sha.slice(0, 7) : '-'}
          </span>
        ),
      },
      {
        id: 'when',
        header: t('history.columns.when'),
        priority: 2,
        cell: (r) => <RelativeTime at={r.created_at} />,
      },
    ],
    [t, stack.pinned_revision],
  )

  return (
    <ResponsiveTable
      rows={revisions}
      columns={columns}
      getRowId={(r) => String(r.number)}
      ariaLabel={t('history.ariaLabel')}
      getRowProps={(r) =>
        r.number === selected ? { 'aria-current': 'true' } : {}
      }
      renderActions={(r) => {
        const pinned = stack.pinned_revision === r.number
        return (
          <span className="flex items-center gap-0.5">
            {canApprove(r, isRoot, currentUser) ? (
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label={t('actions.approveRev', { number: r.number })}
                onClick={() => {
                  onApprove(r)
                }}
              >
                <CheckIcon aria-hidden="true" />
              </Button>
            ) : null}
            {isRoot && r.status === 'pending_approval' ? (
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label={t('actions.rejectRev', { number: r.number })}
                onClick={() => {
                  onReject(r.number)
                }}
              >
                <XIcon aria-hidden="true" />
              </Button>
            ) : null}
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label={t('actions.viewRev', { number: r.number })}
              onClick={() => {
                onView(r.number)
              }}
            >
              <EyeIcon aria-hidden="true" />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label={t('actions.diffRev', { number: r.number })}
              onClick={() => {
                onDiff(r.number)
              }}
            >
              <GitDiffIcon aria-hidden="true" />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label={t('actions.rollbackRev', { number: r.number })}
              disabled={!canRollbackTo(r, stack.latest_revision)}
              onClick={() => {
                onRollback(r.number)
              }}
            >
              <ArrowCounterClockwiseIcon aria-hidden="true" />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label={t(pinned ? 'actions.unpinRev' : 'actions.pinRev', {
                number: r.number,
              })}
              onClick={() => {
                onPin(pinned ? null : r.number)
              }}
            >
              {pinned ? (
                <PushPinSlashIcon aria-hidden="true" />
              ) : (
                <PushPinIcon aria-hidden="true" />
              )}
            </Button>
          </span>
        )
      }}
    />
  )
}
