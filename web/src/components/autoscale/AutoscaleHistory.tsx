import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { autoscaleHistoryQueryOptions } from '../../queries/autoscale'
import { RelativeTime } from '../kit/RelativeTime'

const KINDS = [
  'scaled_up',
  'scaled_down',
  'held',
  'failed',
  'configured',
  'cleared',
] as const
type Kind = (typeof KINDS)[number]

function isKind(k: string): k is Kind {
  return (KINDS as readonly string[]).includes(k)
}

/** AutoscaleHistory lists what the autoscaler decided and why, newest first. */
export function AutoscaleHistory({ appName }: { appName: string }) {
  const { t } = useTranslation('autoscale')
  const { data, isError } = useQuery(autoscaleHistoryQueryOptions(appName))
  return (
    <section aria-labelledby="autoscale-history-title" className="space-y-2">
      <h3 id="autoscale-history-title" className="text-sm font-medium">
        {t('history.title')}
      </h3>
      {isError ? (
        <p className="text-xs text-muted-foreground">
          {t('history.loadError')}
        </p>
      ) : data && data.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t('history.empty')}</p>
      ) : (
        <ul className="divide-y divide-border text-sm">
          {(data ?? []).map((e) => (
            <li
              key={e.id}
              className="flex flex-wrap items-baseline justify-between gap-x-4 py-1.5"
            >
              <span className="min-w-0">
                <span className="font-medium">
                  {isKind(e.kind) ? t(`history.kind.${e.kind}`) : e.kind}
                </span>
                {e.kind === 'scaled_up' || e.kind === 'scaled_down' ? (
                  <span className="ml-2 tabular-nums">
                    {t('history.replicas', { from: e.current, to: e.desired })}
                  </span>
                ) : null}
                <span className="ml-2 text-muted-foreground">{e.detail}</span>
                {e.blocked_by ? (
                  <span className="ml-2 text-xs text-muted-foreground">
                    {t('history.blockedBy', {
                      by: t(`blocked.${e.blocked_by}`, {
                        defaultValue: e.blocked_by,
                      }),
                    })}
                  </span>
                ) : null}
              </span>
              <span className="text-xs text-muted-foreground">
                <RelativeTime at={e.at} />
              </span>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
