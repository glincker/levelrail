import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { siteChecksQueryOptions } from '../../queries/pagesSites'
import { SiteChecksCard } from '../pages/SiteChecksCard'
import { StatTile } from './StatTile'

/** Score and failing counts above the full check list (SiteChecksCard already sorts problems first). */
export function StaticHealth({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('appPages')
  const q = useQuery(siteChecksQueryOptions(name))
  const report = q.data
  const failing = report?.checks.filter((c) => c.status === 'fail').length ?? 0
  const warning = report?.checks.filter((c) => c.status === 'warn').length ?? 0
  return (
    <div className="space-y-4">
      {report ? (
        <section
          aria-label={t('health.summary')}
          className="grid grid-cols-2 gap-3 sm:grid-cols-3"
        >
          <StatTile
            label={t('static.score')}
            value={report.score}
            state={
              failing > 0 ? 'failed' : warning > 0 ? 'degraded' : 'healthy'
            }
          />
          <StatTile
            label={t('health.fail')}
            value={failing}
            state={failing > 0 ? 'failed' : undefined}
          />
          <StatTile
            label={t('health.warn')}
            value={warning}
            state={warning > 0 ? 'degraded' : undefined}
          />
        </section>
      ) : null}
      <SiteChecksCard name={name} />
    </div>
  )
}
