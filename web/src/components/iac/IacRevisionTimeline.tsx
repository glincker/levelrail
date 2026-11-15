import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { RelativeTime } from '@/components/kit'
import { RevisionStatusBadge } from '../stacks/StackIndicators'
import type { StackRevisionSummary } from '../../types/stacks'

export function IacRevisionTimeline({
  revisions,
  pinned,
  selected,
  onSelect,
}: Readonly<{
  revisions: readonly StackRevisionSummary[]
  pinned?: number
  selected: number | null
  onSelect: (n: number) => void
}>) {
  const { t } = useTranslation('settings')
  const { t: ts } = useTranslation('stacks')
  if (revisions.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        {t('iacStudio.stack.noRevisions')}
      </p>
    )
  }
  return (
    <ol
      aria-label={t('iacStudio.stack.timeline')}
      className="flex flex-wrap gap-2"
    >
      {revisions.map((r) => (
        <li key={r.number}>
          <button
            type="button"
            aria-pressed={r.number === selected}
            onClick={() => onSelect(r.number)}
            className="flex flex-col items-start gap-1 rounded-md border bg-card px-2 py-1.5 text-left text-xs transition-colors hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none aria-pressed:border-primary aria-pressed:bg-muted motion-reduce:transition-none"
          >
            <span className="flex items-center gap-1.5">
              <span className="font-mono font-medium">
                {ts('detail.revisionShort', { number: r.number })}
              </span>
              {pinned === r.number ? (
                <Badge variant="info">{ts('history.pinnedMark')}</Badge>
              ) : null}
              {r.rollback_of ? (
                <Badge variant="muted">
                  {ts('history.rollbackOf', { number: r.rollback_of })}
                </Badge>
              ) : null}
            </span>
            <RevisionStatusBadge status={r.rejected ? 'failed' : r.status} />
            <span className="text-muted-foreground">
              {r.actor} <RelativeTime at={r.created_at} />
            </span>
          </button>
        </li>
      ))}
    </ol>
  )
}
