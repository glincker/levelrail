import { useTranslation } from 'react-i18next'
import { StatusPill } from '@/components/kit'
import { formatDurationMs } from '../../lib/deployDuration'
import { barTone, type TimelineDeploy } from '../../lib/deployTimeline'

const SHA_LENGTH = 7

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-[11px] text-muted-foreground">{label}</dt>
      <dd className="truncate text-sm">{value}</dd>
    </div>
  )
}

export function TimelineHoverCard({
  deploy,
}: {
  deploy: TimelineDeploy | null
}) {
  const { t } = useTranslation('deployTimeline')
  return (
    <section
      aria-label={t('card.title')}
      aria-live="polite"
      className="min-h-24 rounded-lg border border-border bg-card p-3"
    >
      {deploy ? (
        <>
          <div className="mb-2 flex flex-wrap items-center gap-2">
            <span className="text-sm font-medium">{deploy.app}</span>
            <StatusPill
              size="sm"
              tone={barTone(deploy)}
              label={t(`status.${deploy.status}` as 'status.ready', {
                defaultValue: deploy.status,
              })}
            />
            {deploy.rollback_of ? (
              <span className="text-xs text-muted-foreground">
                {t('card.rollbackOf', { id: deploy.rollback_of })}
              </span>
            ) : null}
          </div>
          {deploy.commit_message ? (
            <p className="mb-2 truncate text-sm text-muted-foreground">
              {deploy.commit_message}
            </p>
          ) : null}
          <dl className="grid grid-cols-2 gap-x-4 gap-y-2 sm:grid-cols-4">
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
        </>
      ) : (
        <p className="text-sm text-muted-foreground">{t('card.hint')}</p>
      )}
    </section>
  )
}
