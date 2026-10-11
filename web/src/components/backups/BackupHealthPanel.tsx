import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  LockKeyIcon,
  LockKeyOpenIcon,
  ShieldCheckIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toast } from '@/components/ui/toast'
import { formatBytes, formatDate } from '../../lib/format'
import {
  useBackupHealth,
  useRefreshBackupProtection,
  useStartBackupDrill,
} from '../../queries/backupProtection'
import type { ResourceHealth } from '../../types/backupProtection'
import {
  DATABASES_GROUP,
  groupHealth,
  sortByState,
  stateBadgeVariant,
} from './backupHealthHelpers'
import { RestoreWizardDialog } from './RestoreWizardDialog'
import { VolumePolicyDialog } from './VolumePolicyDialog'

function HealthRow({ resource }: { resource: ResourceHealth }) {
  const { t } = useTranslation('backups')
  const drill = useStartBackupDrill()
  const [restoreOpen, setRestoreOpen] = useState(false)
  const [policyOpen, setPolicyOpen] = useState(false)
  const isVolume = resource.kind === 'volume'
  const app = resource.app_name ?? ''
  const lastBackupId = resource.last_backup?.id

  function runDrill() {
    if (!lastBackupId) return
    drill.mutate(lastBackupId, {
      onSuccess: () => {
        toast.add({
          title: t('health.drillStarted'),
          description: t('health.drillStartedDescription'),
          type: 'success',
        })
      },
      onError: (err) => {
        toast.add({
          title: t('health.drillFailedToast'),
          description: err.message,
          type: 'error',
        })
      },
    })
  }

  return (
    <TableRow>
      <TableCell className="font-medium">
        <div className="flex flex-col gap-1">
          {isVolume ? (
            resource.resource_name
          ) : (
            <Link
              to="/databases/$name/overview"
              params={{ name: resource.resource_name }}
              className="underline-offset-2 hover:underline"
            >
              {resource.resource_name}
            </Link>
          )}
          <span className="flex items-center gap-1 text-xs font-normal text-muted-foreground">
            {resource.encrypted ? (
              <LockKeyIcon className="size-3" aria-hidden="true" />
            ) : (
              <LockKeyOpenIcon className="size-3" aria-hidden="true" />
            )}
            {resource.encrypted
              ? t('health.encrypted')
              : t('health.notEncrypted')}
            {' · '}
            {t('health.backups', { count: resource.backup_count })}
          </span>
        </div>
      </TableCell>
      <TableCell>
        <div className="flex flex-col gap-1">
          <Badge variant={stateBadgeVariant(resource.state)}>
            {t(`health.state.${resource.state}`)}
          </Badge>
          {resource.state_reason ? (
            <span className="max-w-[16rem] text-xs text-muted-foreground">
              {resource.state_reason}
            </span>
          ) : null}
        </div>
      </TableCell>
      <TableCell className="text-muted-foreground">
        {resource.last_backup
          ? formatDate(resource.last_backup.at, '-')
          : t('health.none')}
      </TableCell>
      <TableCell>
        {resource.last_verified_restore ? (
          <span className="text-foreground">
            {formatDate(resource.last_verified_restore.at, '-')}
          </span>
        ) : (
          <span className="text-muted-foreground">{t('health.never')}</span>
        )}
        {resource.last_drill?.status === 'failed' ? (
          <span
            className="block text-xs text-destructive"
            title={resource.last_drill.error}
          >
            {t('health.drillFailed')}
          </span>
        ) : null}
      </TableCell>
      <TableCell className="text-muted-foreground">
        {formatBytes(resource.total_bytes)}
      </TableCell>
      <TableCell className="text-muted-foreground">
        {resource.next_run
          ? formatDate(resource.next_run, '-')
          : t('health.notScheduled')}
      </TableCell>
      <TableCell>
        <div className="flex flex-wrap items-center gap-2">
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={!lastBackupId || drill.isPending}
            onClick={runDrill}
          >
            <ShieldCheckIcon className="size-3.5" aria-hidden="true" />
            {t('health.runDrill')}
          </Button>
          {isVolume ? (
            <>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => {
                  setRestoreOpen(true)
                }}
              >
                {t('health.restore')}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="ghost"
                onClick={() => {
                  setPolicyOpen(true)
                }}
              >
                {t('health.policy')}
              </Button>
              <RestoreWizardDialog
                app={app}
                volume={resource.resource_name}
                open={restoreOpen}
                onOpenChange={setRestoreOpen}
              />
              <VolumePolicyDialog
                app={app}
                volume={resource.resource_name}
                open={policyOpen}
                onOpenChange={setPolicyOpen}
              />
            </>
          ) : (
            <Button
              size="sm"
              variant="outline"
              render={
                <Link
                  to="/databases/$name/overview"
                  params={{ name: resource.resource_name }}
                />
              }
              nativeButton={false}
            >
              {t('health.restoreDatabase')}
            </Button>
          )}
        </div>
      </TableCell>
    </TableRow>
  )
}

function BucketProtection() {
  const { t } = useTranslation('backups')
  const health = useBackupHealth()
  const refresh = useRefreshBackupProtection()
  const targets = health.data?.targets ?? []
  const warnings = targets.filter((x) => x.warning || x.probe_error)

  return (
    <div className="space-y-3">
      {health.data ? (
        <Alert>
          <ShieldCheckIcon className="size-4" aria-hidden="true" />
          <AlertDescription>
            {health.data.encryption.enabled
              ? t('encryption.on', {
                  key: health.data.encryption.recipient ?? '',
                })
              : t('encryption.off')}
            {health.data.encryption.enabled
              ? ` ${t('encryption.keyHint')}`
              : ''}
          </AlertDescription>
        </Alert>
      ) : null}
      {warnings.map((w) => (
        <Alert key={w.target_id} variant="destructive">
          <WarningIcon className="size-4" aria-hidden="true" />
          <AlertDescription>
            <strong>{t('protection.target', { id: w.target_id })}</strong>{' '}
            {w.probe_error
              ? t('protection.probeError', { error: w.probe_error })
              : w.warning}
          </AlertDescription>
        </Alert>
      ))}
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={refresh.isPending}
        onClick={() => {
          refresh.mutate(undefined, {
            onError: (err) => {
              toast.add({
                title: t('protection.checkFailed'),
                description: err.message,
                type: 'error',
              })
            },
          })
        }}
      >
        {refresh.isPending ? t('protection.checking') : t('protection.check')}
      </Button>
    </div>
  )
}

/** BackupHealthPanel shows every backed up resource grouped by app, with last backup and last verified restore. */
export function BackupHealthPanel() {
  const { t } = useTranslation('backups')
  const health = useBackupHealth()
  const groups = groupHealth(health.data?.resources ?? [])

  return (
    <section className="space-y-4" aria-labelledby="backup-health-title">
      <div>
        <h2 id="backup-health-title" className="text-base font-medium">
          {t('health.title')}
        </h2>
        <p className="text-sm text-muted-foreground">
          {t('health.description')}
        </p>
      </div>
      <BucketProtection />
      {health.isError ? (
        <p className="text-sm text-destructive">
          {t('health.loadFailed', { error: health.error.message })}
        </p>
      ) : groups.length === 0 && !health.isPending ? (
        <p className="text-sm text-muted-foreground">{t('health.empty')}</p>
      ) : (
        groups.map((g) => (
          <div key={g.key} className="space-y-2">
            <h3 className="text-sm font-medium text-foreground">
              {g.key === DATABASES_GROUP ? (
                t('health.databasesGroup')
              ) : (
                <Link
                  to="/apps/$name/volumes"
                  params={{ name: g.app ?? '' }}
                  className="underline-offset-2 hover:underline"
                >
                  {t('health.appGroup', { name: g.app ?? '' })}
                </Link>
              )}
            </h3>
            <div className="overflow-x-auto rounded-lg border border-border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('health.columns.resource')}</TableHead>
                    <TableHead>{t('health.columns.state')}</TableHead>
                    <TableHead>{t('health.columns.lastBackup')}</TableHead>
                    <TableHead>{t('health.columns.lastRestore')}</TableHead>
                    <TableHead>{t('health.columns.size')}</TableHead>
                    <TableHead>{t('health.columns.nextRun')}</TableHead>
                    <TableHead>{t('health.columns.actions')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {sortByState(g.resources).map((r) => (
                    <HealthRow
                      key={`${r.kind}:${r.app_name ?? ''}:${r.resource_name}`}
                      resource={r}
                    />
                  ))}
                </TableBody>
              </Table>
            </div>
          </div>
        ))
      )}
    </section>
  )
}
