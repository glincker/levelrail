import { useTranslation } from 'react-i18next'
import { Skeleton } from '@/components/ui/skeleton'
import { InfoTip, type StatusState } from '@/components/kit'
import { useAppHealthScore } from '../../queries/healthScore'
import type { AppDetail } from '../../types/appDetail'
import type { HealthScoreStatus } from '../../types/healthScore'
import { StatTile } from './StatTile'

const STATE: Record<HealthScoreStatus, StatusState> = {
  pass: 'healthy',
  warn: 'degraded',
  fail: 'failed',
}

/** Probe state plus one dot per readiness area, problems first; no invented history. */
export function AppHealthSummary({ app }: Readonly<{ app: AppDetail }>) {
  const { t } = useTranslation('appPages')
  const q = useAppHealthScore(app.name)
  const cats = [...(q.data?.categories ?? [])].sort(
    (a, b) => Number(b.status === 'fail') - Number(a.status === 'fail'),
  )
  const problems = cats.filter((c) => c.status !== 'pass')
  const probeOn = (p?: object | null) => Boolean(p)
  return (
    <section aria-label={t('health.summary')} className="space-y-3">
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
        {q.data ? (
          <StatTile
            label={t('health.score')}
            value={t(`health.${q.data.status}`)}
            state={STATE[q.data.status]}
          />
        ) : (
          <Skeleton className="h-[4.5rem]" />
        )}
        <StatTile
          label={t('health.readiness')}
          value={
            probeOn(app.health?.readiness)
              ? t('health.probeOn')
              : t('health.probeOff')
          }
          state={probeOn(app.health?.readiness) ? 'healthy' : 'stopped'}
        />
        <StatTile
          label={t('health.liveness')}
          value={
            probeOn(app.health?.liveness)
              ? t('health.probeOn')
              : t('health.probeOff')
          }
          state={probeOn(app.health?.liveness) ? 'healthy' : 'stopped'}
        />
      </div>
      {cats.length > 0 ? (
        <div className="space-y-2 rounded-lg border border-border bg-card p-3">
          <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
            {t('health.categories')}
            <InfoTip>{t('health.scoreTip')}</InfoTip>
          </p>
          <ul className="flex gap-1" aria-label={t('health.categories')}>
            {cats.map((c) => (
              <li
                key={c.key}
                title={`${c.label}: ${c.reason}`}
                className={`h-2 flex-1 rounded-full ${
                  c.status === 'pass'
                    ? 'bg-tone-success-solid'
                    : c.status === 'warn'
                      ? 'bg-tone-warning-solid'
                      : 'bg-tone-danger-solid'
                }`}
              >
                <span className="sr-only">
                  {c.label}: {t(`health.${c.status}`)}
                </span>
              </li>
            ))}
          </ul>
          {problems.length > 0 ? (
            <ul className="space-y-1 text-sm">
              {problems.map((c) => (
                <li key={c.key} className="flex items-baseline gap-2">
                  <span className="font-medium">{c.label}</span>
                  <span className="truncate text-xs text-muted-foreground">
                    {c.reason}
                  </span>
                </li>
              ))}
            </ul>
          ) : null}
        </div>
      ) : null}
    </section>
  )
}
