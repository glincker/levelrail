import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { PlayGlassIcon, ScrollGlassIcon } from '@thesvg/ui-react'
import { Button, buttonVariants } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { RelativeTime, StatusPill, type Tone } from '@/components/kit'
import { formatBytes } from '@/lib/format'
import { databaseHealth, type HealthState } from '@/lib/databaseHealth'
import { engineHasBackups } from '@/lib/databaseCatalog'
import { useBackupHistory } from '../../../queries/backupHistory'
import { useStartDatabase } from '../../../queries/databases'
import type { DatabaseResource } from '../../../types/databaseDetail'
import type { ReconcileCondition } from '../../../types/deploy'
import { ConnectDrawer } from '../ConnectDrawer'

const TONE_BY_STATE: Record<HealthState, Tone> = {
  healthy: 'success',
  settingUp: 'info',
  stopped: 'neutral',
  attention: 'danger',
}

function Stat({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="min-w-0 space-y-1">
      <dt className="text-xs text-muted-foreground uppercase">{label}</dt>
      <dd className="text-sm font-medium text-foreground">{children}</dd>
    </div>
  )
}

export function StatusHeaderSkeleton() {
  return (
    <Card aria-hidden="true">
      <CardContent className="space-y-4">
        <Skeleton className="h-6 w-32 rounded-full" />
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          {Array.from({ length: 4 }, (_, i) => (
            <div key={i} className="space-y-1">
              <Skeleton className="h-3 w-16" />
              <Skeleton className="h-4 w-24" />
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  )
}

export function StatusHeader({
  database,
  conditions,
}: {
  database: DatabaseResource
  conditions: ReconcileCondition[]
}) {
  const { t } = useTranslation('databases')
  const health = databaseHealth(database, conditions)
  const hasBackups = engineHasBackups(database.engine)
  const history = useBackupHistory(database.name)
  const start = useStartDatabase()
  const lastOk = history.data?.find((b) => b.status === 'succeeded')

  function runStart() {
    start.mutate(database.name, {
      onSuccess: () => {
        toast.add({
          title: t('detail.startToast', { name: database.name }),
          type: 'success',
        })
      },
      onError: (error) => {
        toast.add({ title: error.message, type: 'error' })
      },
    })
  }

  return (
    <Card>
      <CardContent className="space-y-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0 space-y-1">
            <StatusPill
              tone={TONE_BY_STATE[health.state]}
              label={t(`status.${health.state}`)}
              live={health.state === 'settingUp'}
            />
            <p className="text-sm text-muted-foreground">
              {t(`status.${health.state}Hint`)}
            </p>
          </div>
          {health.action === 'connect' ? (
            <ConnectDrawer database={database} />
          ) : null}
          {health.action === 'start' ? (
            <Button size="sm" disabled={start.isPending} onClick={runStart}>
              <PlayGlassIcon size="1.4em" aria-hidden="true" />
              {t('detail.start')}
            </Button>
          ) : null}
          {health.action === 'logs' ? (
            <Link
              to="/databases/$name/logs"
              params={{ name: database.name }}
              className={buttonVariants({ size: 'sm' })}
            >
              <ScrollGlassIcon size="1.4em" aria-hidden="true" />
              {t('status.viewLogs')}
            </Link>
          ) : null}
        </div>
        <dl className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          <Stat label={t('connection.engine')}>
            <span className="capitalize">{database.engine}</span>{' '}
            <span className="font-mono text-muted-foreground">
              {database.version}
            </span>
          </Stat>
          {hasBackups ? (
            <>
              <Stat label={t('status.lastBackup')}>
                {history.isPending ? (
                  <Skeleton className="h-4 w-20" />
                ) : lastOk?.finished_at ? (
                  <RelativeTime at={lastOk.finished_at} />
                ) : (
                  <span className="text-muted-foreground">
                    {t('status.noBackupYet')}
                  </span>
                )}
              </Stat>
              <Stat label={t('status.backupSize')}>
                {lastOk?.size_bytes ? (
                  formatBytes(lastOk.size_bytes)
                ) : (
                  <span className="text-muted-foreground">
                    {t('status.none')}
                  </span>
                )}
              </Stat>
              <Stat label={t('status.schedule')}>
                {database.backup_schedule ? (
                  <span className="font-mono">{database.backup_schedule}</span>
                ) : (
                  <span className="text-muted-foreground">
                    {t('status.notScheduled')}
                  </span>
                )}
              </Stat>
            </>
          ) : null}
        </dl>
      </CardContent>
    </Card>
  )
}
