import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  CircleXGlassIcon,
  InfoGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { RelativeTime } from '@/components/kit'
import { eventKindKey, unixToDate } from '../../lib/nodeTrust'
import type { NodeSecurityEvent, SecurityOutcome } from '../../types/nodeTrust'
import { useNodeSecurityEvents } from '../../queries/nodeTrust'

const OUTCOME_ICON: Record<SecurityOutcome, typeof InfoGlassIcon> = {
  accepted: CircleCheckGlassIcon,
  rejected: CircleXGlassIcon,
  failed: CircleXGlassIcon,
  warning: TriangleAlertGlassIcon,
  info: InfoGlassIcon,
}

const OUTCOME_TONE: Record<SecurityOutcome, string> = {
  accepted: 'text-tone-success',
  rejected: 'text-tone-danger',
  failed: 'text-tone-danger',
  warning: 'text-tone-warning',
  info: 'text-muted-foreground',
}

function EventRow({ e }: { e: NodeSecurityEvent }) {
  const { t } = useTranslation('nodes')
  const Icon = OUTCOME_ICON[e.outcome]
  const reason = e.reason
    ? t(`trust.reasons.${e.reason}`, { defaultValue: e.reason })
    : ''
  const expires = unixToDate(e.not_after)
  return (
    <li className="flex items-start gap-2 py-2">
      <Icon
        className={`mt-0.5 size-5 shrink-0 ${OUTCOME_TONE[e.outcome]}`}
        aria-label={t(`trust.outcome.${e.outcome}`)}
      />
      <div className="min-w-0 flex-1">
        <p className="text-sm break-words text-foreground">
          {t(`trust.events.${eventKindKey(e)}`, {
            reason,
            kind: e.kind,
            subject: e.subject ?? '',
            actor: e.actor ?? '',
            token: e.token_id ?? '',
            serial: e.serial ?? '',
            expires: expires ? expires.toLocaleDateString() : '',
          })}
          {e.count > 1 ? ` ×${e.count}` : ''}
        </p>
        <p className="text-xs break-all text-muted-foreground">
          <RelativeTime at={e.last_at} />
          {e.source_ip ? ` · ${e.source_ip}` : ''}
          {e.message ? ` · ${e.message}` : ''}
          {e.fingerprint ? ` · ${e.fingerprint.slice(0, 16)}` : ''}
        </p>
      </div>
    </li>
  )
}

export function NodeSecurityTimeline({ nodeId }: { nodeId: string }) {
  const { t } = useTranslation('nodes')
  const [rejectedOnly, setRejectedOnly] = useState(false)
  const events = useNodeSecurityEvents(nodeId, rejectedOnly, true)
  return (
    <section className="space-y-2" aria-labelledby={`sec-${nodeId}`}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 id={`sec-${nodeId}`} className="text-sm font-medium">
          {t('trust.timeline')}
        </h3>
        <div className="flex gap-1">
          {[false, true].map((only) => (
            <Button
              key={String(only)}
              type="button"
              size="xs"
              variant={rejectedOnly === only ? 'secondary' : 'outline'}
              aria-pressed={rejectedOnly === only}
              onClick={() => {
                setRejectedOnly(only)
              }}
            >
              {t(only ? 'trust.filterRejected' : 'trust.filterAll')}
            </Button>
          ))}
        </div>
      </div>
      {events.isError ? (
        <p className="text-sm text-tone-danger">{t('trust.eventsError')}</p>
      ) : events.data && events.data.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t(rejectedOnly ? 'trust.noRejected' : 'trust.noEvents')}
        </p>
      ) : (
        <ul className="divide-y divide-border">
          {(events.data ?? []).map((e) => (
            <EventRow key={e.id} e={e} />
          ))}
        </ul>
      )}
    </section>
  )
}
