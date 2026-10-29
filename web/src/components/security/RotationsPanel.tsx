import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { RefreshCwGlassIcon } from '@thesvg/ui-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { buttonVariants } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { RotationStatusBadge } from '../env/SecretRotationSection'
import {
  useRotationOverview,
  type RotationPolicy,
} from '../../queries/secretRotation'
import { formatDate } from '../../lib/format'

function weight(p: RotationPolicy): number {
  if (p.state === 'restart_failed' || p.state === 'failed') return 0
  if (p.status === 'overdue') return 1
  if (p.status === 'due_soon') return 2
  return 3
}

export function RotationsPanel() {
  const { t } = useTranslation('secretRotation')
  const { data } = useRotationOverview()
  const rows = [...(data ?? [])].sort((a, b) => weight(a) - weight(b))
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('overview.title')}</CardTitle>
        <CardDescription>{t('overview.description')}</CardDescription>
      </CardHeader>
      <CardContent>
        {rows.length === 0 ? (
          <EmptyState
            icon={<RefreshCwGlassIcon size="1.2em" />}
            title={t('overview.title')}
            description={t('overview.empty')}
          />
        ) : (
          <ul className="divide-y divide-border rounded-lg border border-border">
            {rows.map((p) => (
              <li
                key={`${p.app ?? ''}/${p.key}`}
                className="flex flex-wrap items-center justify-between gap-3 px-3 py-2"
              >
                <div className="min-w-0 space-y-0.5">
                  <p className="truncate font-mono text-sm">
                    {p.app}/{p.key}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {t('columns.nextDue')}:{' '}
                    {formatDate(p.next_due_at, t('unknownDate'))} ·{' '}
                    {t('columns.owner')}: {p.owner}
                  </p>
                  {p.last_error ? (
                    <p role="alert" className="text-xs text-destructive">
                      {p.last_error}
                    </p>
                  ) : null}
                </div>
                <div className="flex items-center gap-2">
                  <RotationStatusBadge policy={p} />
                  <Link
                    to="/apps/$name/environment"
                    params={{ name: p.app ?? '' }}
                    className={buttonVariants({
                      variant: 'outline',
                      size: 'sm',
                    })}
                  >
                    {t('overview.open')}
                  </Link>
                </div>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
