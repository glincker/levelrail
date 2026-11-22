import { useTranslation } from 'react-i18next'
import { formatAbsoluteTime } from '../../lib/relativeTime'
import { useCloudInstanceEvents } from '../../queries/cloudInstanceEvents'

// Rendered only when the provider's capability matrix lists events.
export function CloudInstanceEvents({
  accountId,
  instanceId,
}: Readonly<{ accountId: string; instanceId: string }>) {
  const { t } = useTranslation('cloudAccounts')
  const q = useCloudInstanceEvents(accountId, instanceId, true)
  const rows = q.data?.events ?? []
  return (
    <section className="space-y-2" aria-label={t('instances.events.title')}>
      <h3 className="text-xs font-semibold text-muted-foreground">
        {t('instances.events.title')}
      </h3>
      {q.isPending ? (
        <p role="status" className="text-sm text-muted-foreground">
          {t('section.loading')}
        </p>
      ) : null}
      {q.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {q.error instanceof Error ? q.error.message : String(q.error)}
        </p>
      ) : null}
      {q.data && rows.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('instances.events.none')}
        </p>
      ) : null}
      <ul className="space-y-1">
        {rows.map((e) => (
          <li
            key={`${e.time}-${e.kind}-${e.status}`}
            className="flex flex-wrap gap-x-2 text-xs"
          >
            <span className="font-mono text-muted-foreground">
              {formatAbsoluteTime(e.time)}
            </span>
            <span className="font-medium">{e.kind}</span>
            <span className="text-muted-foreground">{e.status}</span>
            {e.message ? <span>{e.message}</span> : null}
          </li>
        ))}
      </ul>
    </section>
  )
}
