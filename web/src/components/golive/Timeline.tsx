import { useTranslation } from 'react-i18next'
import { ClockIcon, WarningCircleIcon } from '@phosphor-icons/react/dist/ssr'
import type { TimelineEvent } from '../../queries/goLiveChecklist'

export function Timeline({ events }: { events: TimelineEvent[] }) {
  const { t } = useTranslation('golive')
  return (
    <section aria-label={t('timeline.title')} className="space-y-2">
      <h3 className="text-sm font-semibold">{t('timeline.title')}</h3>
      {events.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('timeline.empty')}</p>
      ) : (
        <ol className="space-y-2">
          {events.map((e) => (
            <li key={e.id} className="flex items-start gap-2 text-sm">
              {e.failed ? (
                <WarningCircleIcon
                  className="mt-0.5 size-4 shrink-0 text-tone-danger"
                  aria-hidden="true"
                />
              ) : (
                <ClockIcon
                  className="mt-0.5 size-4 shrink-0 text-muted-foreground"
                  aria-hidden="true"
                />
              )}
              <div className="min-w-0">
                <p className="font-medium">
                  {e.title}
                  {e.future ? ` (${String(t('timeline.scheduled'))})` : ''}
                </p>
                {e.detail ? (
                  <p className="text-xs text-muted-foreground">{e.detail}</p>
                ) : null}
                <time dateTime={e.at} className="text-xs text-muted-foreground">
                  {new Date(e.at).toLocaleString()}
                </time>
              </div>
            </li>
          ))}
        </ol>
      )}
    </section>
  )
}
