import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ActivityGlassIcon } from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
import { useDatabase } from '../../../queries/databases'
import { engineHasQueryTabs } from '../monitoring/chartRegistry'

const TABS = [
  { to: '/databases/$name/metrics', key: 'metrics' },
  { to: '/databases/$name/slow-queries', key: 'slow' },
  { to: '/databases/$name/logs', key: 'logs' },
] as const

// Honest placeholders: no endpoint backs these yet, so they stay disabled.
const SOON = ['active', 'perf'] as const

function instanceState(db: {
  suspended?: boolean
  provision_state?: string
}): 'active' | 'stopped' | 'provisioning' {
  if (db.provision_state) return 'provisioning'
  return db.suspended ? 'stopped' : 'active'
}

// Metrics, logs and slow queries stay separate routes (deep links keep
// working) but read as one Monitoring section through this shared header.
export function MonitoringTabs({ name }: { name: string }) {
  const { t } = useTranslation('dbMonitoring')
  const { data: db } = useDatabase(name)
  const state = instanceState(db)
  return (
    <header className="mb-3 flex flex-wrap items-center justify-between gap-x-4 gap-y-2 border-b border-border">
      <nav aria-label={t('tabs.aria')} className="-mb-px flex flex-wrap gap-1">
        {TABS.map((tab) => (
          <Link
            key={tab.key}
            to={tab.to}
            params={{ name }}
            className="border-b-2 border-transparent px-3 py-2 text-sm text-muted-foreground transition-colors hover:text-foreground"
            activeProps={{
              className: 'border-primary font-medium text-foreground',
              'aria-current': 'page',
            }}
          >
            {t(`tabs.${tab.key}`)}
          </Link>
        ))}
        {engineHasQueryTabs(db.engine)
          ? SOON.map((key) => (
              <button
                key={key}
                type="button"
                disabled
                title={t('tabs.soon')}
                className="inline-flex items-center gap-1.5 border-b-2 border-transparent px-3 py-2 text-sm text-muted-foreground/60"
              >
                {t(`tabs.${key}`)}
                <span className="rounded bg-muted px-1 text-xs">
                  {t('tabs.soonBadge')}
                </span>
              </button>
            ))
          : null}
      </nav>
      <p
        className="mb-1 inline-flex items-center gap-2 rounded-md border border-border bg-card px-2.5 py-1 text-xs"
        aria-label={t('instance.label')}
      >
        <ActivityGlassIcon
          className="size-4 text-muted-foreground"
          aria-hidden="true"
        />
        <span className="font-medium text-foreground">{name}</span>
        <span
          className={cn(
            'inline-flex items-center gap-1 rounded-full px-1.5 py-0.5 font-medium',
            state === 'active'
              ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400'
              : 'bg-muted text-muted-foreground',
          )}
        >
          <span
            className={cn(
              'size-1.5 rounded-full',
              state === 'active' ? 'bg-emerald-500' : 'bg-muted-foreground',
            )}
            aria-hidden="true"
          />
          {t(`instance.${state}`)}
        </span>
      </p>
    </header>
  )
}
