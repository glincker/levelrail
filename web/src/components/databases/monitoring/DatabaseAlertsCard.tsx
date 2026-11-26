import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { buttonVariants } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { RelativeTime, StatusPill } from '@/components/kit'
import { cn } from '@/lib/utils'
import {
  thresholdChips,
  type DatabaseAlertRow,
} from '../../../lib/databaseHealthAlerts'
import { useDatabaseAlerts } from '../../../queries/databaseAlerts'
import { useRuntimeSettings } from '../../../queries/runtimeSettings'
import type { RuntimeSetting } from '../../../types/runtimeSettings'
import { alertKindLabel } from '../../alertKindCopy'

export const DATABASE_ALERTS_ANCHOR = 'database-alerts'

export function DatabaseAlertsCardView({
  rows,
  settings,
  loading,
}: {
  rows: DatabaseAlertRow[]
  settings: RuntimeSetting[]
  loading?: boolean
}) {
  const { t } = useTranslation('dbMonitoring')
  const { t: tObs } = useTranslation('observability')
  const chipText = (id: string, value: string) => {
    switch (id) {
      case 'diskWarn':
        return t('alerts.chips.diskWarn', { value })
      case 'diskCritical':
        return t('alerts.chips.diskCritical', { value })
      case 'holdFor':
        return t('alerts.chips.holdFor', { value })
      case 'connections':
        return t('alerts.chips.connections', { value })
      case 'backupWindow':
        return t('alerts.chips.backupWindow', { value })
      default:
        return t('alerts.chips.unprotected', { value })
    }
  }
  const linkClass = cn(buttonVariants({ variant: 'ghost', size: 'sm' }))
  return (
    <Card id={DATABASE_ALERTS_ANCHOR} className="scroll-mt-20">
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
        <CardTitle>{t('alerts.title')}</CardTitle>
        <div className="flex items-center gap-1">
          <Link
            to="/settings/security-runtime"
            search={{ category: 'observability', q: 'alerts.database' }}
            className={linkClass}
          >
            {t('alerts.editThresholds')}
          </Link>
          <Link to="/alerts" className={linkClass}>
            {t('alerts.allRules')}
          </Link>
        </div>
      </CardHeader>
      <CardContent>
        {loading ? (
          <Skeleton className="h-16 w-full" />
        ) : rows.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('alerts.empty')}</p>
        ) : (
          <ul className="divide-y divide-border">
            {rows.map((row) => {
              const chips = thresholdChips(row.kind, row.severity, settings)
              return (
                <li
                  key={row.id}
                  className="flex flex-wrap items-center gap-x-3 gap-y-1 py-2 text-sm"
                >
                  <StatusPill
                    size="sm"
                    tone={
                      row.state === 'firing'
                        ? row.severity === 'critical'
                          ? 'danger'
                          : 'warning'
                        : row.state === 'ok'
                          ? 'success'
                          : 'neutral'
                    }
                    label={t(`alerts.state.${row.state}`)}
                  />
                  <span className="font-medium">
                    {alertKindLabel(tObs, row.kind)}
                  </span>
                  <span className="text-xs text-muted-foreground">
                    {t(
                      `alerts.severity.${row.severity === 'critical' ? 'critical' : 'warning'}`,
                    )}
                  </span>
                  <span className="flex flex-wrap gap-1">
                    {chips.map((c) => (
                      <Badge key={c.id} variant="outline">
                        {chipText(c.id, c.value)}
                      </Badge>
                    ))}
                  </span>
                  {row.state === 'firing' ? (
                    <span className="basis-full text-xs text-muted-foreground">
                      {row.since ? (
                        <>
                          {t('alerts.since')} <RelativeTime at={row.since} />
                        </>
                      ) : null}
                      {row.detail ? ` ${row.detail}` : null}
                      {row.unattributed ? ` ${t('alerts.unattributed')}` : null}
                    </span>
                  ) : null}
                </li>
              )
            })}
          </ul>
        )}
        <p className="mt-2 text-xs text-muted-foreground">
          {t('alerts.scopeNote')}
        </p>
      </CardContent>
    </Card>
  )
}

export function DatabaseAlertsCard({ databaseName }: { databaseName: string }) {
  const { rows, isLoading } = useDatabaseAlerts(databaseName)
  const settings = useRuntimeSettings()
  return (
    <DatabaseAlertsCardView
      rows={rows}
      settings={settings.data?.settings ?? []}
      loading={isLoading}
    />
  )
}
