import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useVirtualizer } from '@tanstack/react-virtual'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  type AuthGateDecision,
  type AuthGateEvent,
  useAuthGateEvents,
} from '../queries/domainAuthGate'

const RANGES = ['1h', '24h', '7d'] as const
type Range = (typeof RANGES)[number]

export const ACCESS_LOG_VIRTUALIZE_OVER = 50

const GRID =
  'grid grid-cols-[9rem_minmax(8rem,1fr)_8rem_minmax(6rem,1fr)_7rem_4rem] gap-2 px-3'

const DECISION_VARIANT: Record<
  AuthGateDecision,
  'success' | 'destructive' | 'muted'
> = {
  allowed: 'success',
  token_exchange: 'success',
  denied: 'destructive',
  login_redirect: 'muted',
  token_issued: 'muted',
  session_revoked: 'muted',
}

function AccessLogRow({ event }: { event: AuthGateEvent }) {
  const { t, i18n } = useTranslation('domains')
  return (
    <div
      role="row"
      className={`${GRID} items-center border-b border-border py-2 text-xs`}
    >
      <span role="cell" className="tabular-nums text-muted-foreground">
        {new Date(event.at).toLocaleString(i18n.language)}
      </span>
      <span role="cell" className="truncate">
        {event.user_email || event.user_id || t('authGate.accessLog.anonymous')}
      </span>
      <span role="cell">
        <Badge variant={DECISION_VARIANT[event.decision]}>
          {t(`authGate.accessLog.decision.${event.decision}`)}
        </Badge>
      </span>
      <span role="cell" className="truncate font-mono text-muted-foreground">
        {event.reason}
      </span>
      <span role="cell" className="truncate font-mono">
        {event.ip}
      </span>
      <span role="cell" className="tabular-nums">
        {event.count}
      </span>
    </div>
  )
}

function AccessLogRows({ events }: { events: AuthGateEvent[] }) {
  const parentRef = useRef<HTMLDivElement>(null)
  const virtualizer = useVirtualizer({
    count: events.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 37,
    overscan: 8,
  })
  if (events.length <= ACCESS_LOG_VIRTUALIZE_OVER) {
    return (
      <div role="rowgroup">
        {events.map((e, i) => (
          <AccessLogRow key={`${e.at}:${i}`} event={e} />
        ))}
      </div>
    )
  }
  return (
    <div ref={parentRef} role="rowgroup" className="h-96 overflow-auto">
      <div style={{ height: virtualizer.getTotalSize(), position: 'relative' }}>
        {virtualizer.getVirtualItems().map((v) => {
          const event = events[v.index]
          if (!event) {
            return null
          }
          return (
            <div
              key={v.key}
              data-index={v.index}
              ref={virtualizer.measureElement}
              style={{
                position: 'absolute',
                top: 0,
                left: 0,
                width: '100%',
                transform: `translateY(${v.start}px)`,
              }}
            >
              <AccessLogRow event={event} />
            </div>
          )
        })}
      </div>
    </div>
  )
}

// Who passed, who was turned away and why, for one gated domain.
export function AuthGateAccessLog({
  appName,
  domain,
}: {
  appName: string
  domain: string
}) {
  const { t } = useTranslation('domains')
  const [range, setRange] = useState<Range>('24h')
  const { data, isLoading, isError } = useAuthGateEvents(
    appName,
    domain,
    range,
    true,
  )
  const events = data?.events ?? []

  return (
    <section className="space-y-2" aria-label={t('authGate.accessLog.title')}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h4 className="font-medium">{t('authGate.accessLog.title')}</h4>
        <div
          role="radiogroup"
          aria-label={t('authGate.accessLog.rangeLabel')}
          className="flex gap-1"
        >
          {RANGES.map((r) => (
            <Button
              key={r}
              type="button"
              size="sm"
              role="radio"
              aria-checked={range === r}
              variant={range === r ? 'secondary' : 'ghost'}
              onClick={() => setRange(r)}
            >
              {t(`authGate.accessLog.range.${r}`)}
            </Button>
          ))}
        </div>
      </div>
      {isLoading ? (
        <Skeleton className="h-24 w-full" />
      ) : isError ? (
        <Alert variant="destructive">
          <AlertDescription>
            {t('authGate.accessLog.loadError')}
          </AlertDescription>
        </Alert>
      ) : events.length === 0 ? (
        <p className="text-muted-foreground">{t('authGate.accessLog.empty')}</p>
      ) : (
        <div
          role="table"
          aria-rowcount={events.length + 1}
          className="overflow-x-auto rounded-md border border-border"
        >
          <div className="min-w-[44rem]">
            <div
              role="row"
              className={`${GRID} border-b border-border py-2 text-xs font-medium text-muted-foreground`}
            >
              <span role="columnheader">
                {t('authGate.accessLog.columns.time')}
              </span>
              <span role="columnheader">
                {t('authGate.accessLog.columns.user')}
              </span>
              <span role="columnheader">
                {t('authGate.accessLog.columns.decision')}
              </span>
              <span role="columnheader">
                {t('authGate.accessLog.columns.reason')}
              </span>
              <span role="columnheader">
                {t('authGate.accessLog.columns.ip')}
              </span>
              <span role="columnheader">
                {t('authGate.accessLog.columns.count')}
              </span>
            </div>
            <AccessLogRows events={events} />
          </div>
        </div>
      )}
    </section>
  )
}
