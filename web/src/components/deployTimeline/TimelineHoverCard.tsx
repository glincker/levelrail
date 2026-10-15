import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { StatusPill } from '@/components/kit'
import { formatDurationMs } from '../../lib/deployDuration'
import { barTone, type TimelineDeploy } from '../../lib/deployTimeline'
import { CARD_WIDTH, type CardPlacement } from '../../lib/timelineLayout'

const SHA_LENGTH = 7

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-[11px] text-muted-foreground">{label}</dt>
      <dd className="truncate text-sm">{value}</dd>
    </div>
  )
}

export interface CardAnchor {
  deploy: TimelineDeploy
  placement: CardPlacement
}

/** Floating deploy details anchored to a bar. Decorative: each bar's aria-label carries the same facts. */
export function TimelineHoverCard({ anchor }: { anchor: CardAnchor | null }) {
  const { t } = useTranslation('deployTimeline')
  if (!anchor) return null
  const { deploy, placement } = anchor
  return createPortal(
    <div
      aria-hidden="true"
      data-testid="timeline-card"
      className="pointer-events-none fixed z-50 rounded-lg border border-border bg-popover p-3 text-popover-foreground shadow-lg"
      style={{ left: placement.x, top: placement.y, width: CARD_WIDTH }}
    >
      <div className="mb-1.5 flex flex-wrap items-center gap-2">
        <span className="truncate text-sm font-medium">{deploy.app}</span>
        <StatusPill
          size="sm"
          tone={barTone(deploy)}
          label={t(`status.${deploy.status}` as 'status.ready', {
            defaultValue: deploy.status,
          })}
        />
      </div>
      {deploy.rollback_of ? (
        <p className="mb-1.5 text-xs text-muted-foreground">
          {t('card.rollbackOf', { id: deploy.rollback_of })}
        </p>
      ) : null}
      {deploy.commit_message ? (
        <p className="mb-2 line-clamp-2 text-sm text-muted-foreground">
          {deploy.commit_message}
        </p>
      ) : null}
      <dl className="grid grid-cols-2 gap-x-3 gap-y-2">
        <Field
          label={t('card.commit')}
          value={
            deploy.commit_sha
              ? deploy.commit_sha.slice(0, SHA_LENGTH)
              : t('card.noCommit')
          }
        />
        <Field
          label={t('card.actor')}
          value={deploy.author || t('card.unknownActor')}
        />
        <Field
          label={t('card.duration')}
          value={
            deploy.duration_ms !== null
              ? (formatDurationMs(deploy.duration_ms) ?? '')
              : t('card.running')
          }
        />
        <Field
          label={t('card.strategy')}
          value={deploy.strategy || t('card.noStrategy')}
        />
      </dl>
    </div>,
    document.body,
  )
}
