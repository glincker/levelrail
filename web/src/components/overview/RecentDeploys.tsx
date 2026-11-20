import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { RelativeTime, StatusShape } from '@/components/kit'
import { buttonVariants } from '@/components/ui/button'
import type { DeployAttempt } from '../../types/deployAttempt'
import { DEPLOY_STATE } from './staticStripModel'

const RECENT = 3

/** The last few deploys with the two actions people reach for: logs and rollback. */
export function RecentDeploys({
  appName,
  attempts,
}: {
  appName: string
  attempts: DeployAttempt[]
}) {
  const { t } = useTranslation('appOverview')
  const { t: tq } = useTranslation('appPages')
  const rows = attempts.slice(0, RECENT)
  if (rows.length === 0) return null
  return (
    <section aria-label={t('recent.label')} className="space-y-2">
      <div className="flex items-center justify-between gap-2 px-1">
        <h2 className="text-sm font-medium">{t('recent.label')}</h2>
        <Link
          to="/apps/$name/deploys"
          params={{ name: appName }}
          className="text-xs text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
        >
          {t('recent.all')}
        </Link>
      </div>
      <ul className="divide-y divide-border rounded-lg border border-border bg-card">
        {rows.map((a, i) => (
          <li key={a.id} className="flex items-center gap-3 px-3 py-2">
            <StatusShape state={DEPLOY_STATE[a.status]} />
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium">
                {tq(`quick.deployStatus.${a.status}`)}
                <span className="font-normal text-muted-foreground">
                  {' '}
                  <RelativeTime at={a.started_at} live />
                </span>
              </p>
              <p className="truncate text-xs text-muted-foreground">
                {a.commit_message ??
                  a.commit_sha?.slice(0, 7) ??
                  t(`staticLine.source.${a.source ?? 'other'}`, {
                    defaultValue: t('staticLine.source.other'),
                  })}
              </p>
            </div>
            <Link
              to="/apps/$name/deploys/$deployId/logs"
              params={{ name: appName, deployId: a.id }}
              className={buttonVariants({ variant: 'ghost', size: 'sm' })}
            >
              {t('recent.logs')}
            </Link>
            {a.status === 'succeeded' && i > 0 ? (
              <Link
                to="/apps/$name/deploys"
                params={{ name: appName }}
                className={buttonVariants({ variant: 'outline', size: 'sm' })}
              >
                {t('recent.rollback')}
              </Link>
            ) : null}
          </li>
        ))}
      </ul>
    </section>
  )
}
