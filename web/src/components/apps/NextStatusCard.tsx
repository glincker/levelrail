import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { useAppCache } from '../../queries/cdnCache'
import { useAppVitals } from '../../queries/appVitals'
import { useScheduledTasks } from '../../queries/scheduledTasks'
import type { AppDetail } from '../../types/appDetail'
import { AppBuildCacheRow } from '../AppBuildCacheRow'
import { RelativeTime } from '../kit/RelativeTime'

function Row({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <li className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2 text-sm">
      <span className="w-40 shrink-0 font-medium text-foreground">{label}</span>
      {children}
    </li>
  )
}

// There is no framework field on an app, so Next.js is read from the cache
// advisor's nextjs suggestion or an explicit next_cache setting.
export function NextStatusCard({ app }: { app: AppDetail }) {
  const { t } = useTranslation('resources', { keyPrefix: 'nextStatus' })
  const cache = useAppCache(app.name)
  const vitals = useAppVitals(app.name)
  const tasks = useScheduledTasks(app.name)

  const isNext = cache.data?.suggest_preset === 'nextjs' || !!app.next_cache
  if (!isNext) return null

  const heldUntil = app.previous_release_held_until
  const redis = app.next_cache === 'redis'
  const cronCount = tasks.data?.length ?? 0

  return (
    <section aria-label={t('title')} className="space-y-2">
      <h2 className="text-sm font-medium">{t('title')}</h2>
      <ul className="divide-y divide-border rounded-lg border border-border">
        <Row label={t('skew')}>
          {heldUntil ? (
            <Badge variant="info">
              {t('held')} <RelativeTime at={heldUntil} live />
            </Badge>
          ) : (
            <Badge variant="muted">{t('noneHeld')}</Badge>
          )}
        </Row>
        <Row label={t('cache')}>
          <Badge variant={redis ? 'success' : 'muted'}>
            {redis ? t('redis') : t('off')}
          </Badge>
          <Link
            to="/apps/$name/resources"
            params={{ name: app.name }}
            className="text-primary underline-offset-2 hover:underline"
          >
            {t('configure')}
          </Link>
        </Row>
        <li className="px-3 py-2 text-sm">
          <AppBuildCacheRow appName={app.name} bare />
        </li>
        {vitals.data ? (
          <Row label={t('vitals')}>
            <Badge variant={vitals.data.enabled ? 'success' : 'muted'}>
              {vitals.data.enabled ? t('collecting') : t('notCollecting')}
            </Badge>
          </Row>
        ) : null}
        {tasks.data ? (
          <Row label={t('cron')}>
            <Badge variant={cronCount > 0 ? 'info' : 'muted'}>
              {cronCount > 0
                ? t('cronCount', { count: cronCount })
                : t('noCron')}
            </Badge>
            <Link
              to="/apps/$name/scheduled-tasks"
              params={{ name: app.name }}
              className="text-primary underline-offset-2 hover:underline"
            >
              {t('manage')}
            </Link>
          </Row>
        ) : null}
      </ul>
    </section>
  )
}
