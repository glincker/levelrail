import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArchiveGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import { formatBytes } from '@/lib/modelPreflight'
import { useCanWriteSensitive } from '../../hooks/useCanWriteSensitive'
import {
  runMailBackup,
  useMailBackups,
  type MailBackupJob,
} from '../../queries/mailBackup'
import { useInvalidateMail } from '../../queries/mailMailboxes'
import { MailRestoreDialog } from './MailRestoreDialog'
import { PanelCard, Reason } from './mailPanelUi'

/** Backups for the selected mail app: summary row, run now, list and restore. */
export function MailBackupPanel({ app }: { app: string }) {
  const { t } = useTranslation('mailServer')
  const q = useMailBackups(app)
  const canWrite = useCanWriteSensitive()
  const invalidate = useInvalidateMail()
  const [error, setError] = useState<string | null>(null)
  const [restoreId, setRestoreId] = useState<string | null>(null)
  if (!q.data) return null
  const { backups, job, encrypted, cap_bytes, used_bytes } = q.data
  const running = job?.state === 'running'
  const last = backups[0]
  const pct = cap_bytes > 0 ? Math.min(100, (used_bytes / cap_bytes) * 100) : 0
  const why = canWrite ? undefined : t('backup.needsWrite')
  const stepLabel = (j: MailBackupJob) =>
    t(`backup.steps.${j.step}`, { defaultValue: j.step })

  const start = async () => {
    setError(null)
    try {
      await runMailBackup(app)
      await invalidate()
    } catch (e) {
      setError(e instanceof Error ? e.message : t('backup.failed'))
    }
  }

  return (
    <PanelCard
      label={t('backup.title')}
      title={
        <span className="flex items-center gap-1.5">
          <ArchiveGlassIcon size="1.2em" />
          {t('backup.title')}
        </span>
      }
    >
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-xs">
        <span>
          <span className="text-muted-foreground">{t('backup.last')}: </span>
          {last
            ? new Date(last.created_at).toLocaleString()
            : t('backup.never')}
        </span>
        {last && <span>{formatBytes(last.stored_bytes)}</span>}
        {last && (
          <Badge variant={last.verified ? 'success' : 'warning'}>
            {last.verified ? t('backup.verified') : t('backup.unverified')}
          </Badge>
        )}
        <div className="flex min-w-40 flex-1 flex-col gap-1">
          <Progress value={pct} aria-label={t('backup.usageLabel')} />
          <span className="text-muted-foreground">
            {t('backup.usage', {
              used: formatBytes(used_bytes),
              cap: formatBytes(cap_bytes),
            })}
          </span>
        </div>
        <Reason why={why}>
          <Button
            size="sm"
            disabled={!canWrite || running || !encrypted}
            onClick={() => {
              void start()
            }}
          >
            {running && job
              ? job.kind === 'restore'
                ? t('backup.restoring', { step: stepLabel(job) })
                : t('backup.running', { step: stepLabel(job) })
              : t('backup.runNow')}
          </Button>
        </Reason>
      </div>
      {!encrypted && (
        <p className="text-xs text-tone-warning">{t('backup.notEncrypted')}</p>
      )}
      {job?.state === 'failed' && (
        <p role="alert" className="text-xs text-tone-danger">
          {t('backup.jobFailed', { error: job.error ?? '' })}
        </p>
      )}
      {error && (
        <p role="alert" className="text-xs text-tone-danger">
          {error}
        </p>
      )}
      {backups.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t('backup.empty')}</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="text-muted-foreground">
              <tr>
                <th className="py-1 pr-3 font-medium">
                  {t('backup.col.backup')}
                </th>
                <th className="py-1 pr-3 font-medium">
                  {t('backup.col.size')}
                </th>
                <th className="py-1 pr-3 font-medium">{t('backup.col.mox')}</th>
                <th className="py-1 pr-3 font-medium">
                  {t('backup.col.where')}
                </th>
                <th className="py-1 font-medium">
                  <span className="sr-only">{t('backup.col.actions')}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {backups.map((b) => (
                <tr key={b.id} className="border-t border-border align-top">
                  <td className="py-1.5 pr-3">
                    <span className="font-mono">{b.id}</span>
                    <span className="ml-2 text-muted-foreground">
                      {new Date(b.created_at).toLocaleString()}
                    </span>
                    {b.verified && (
                      <Badge variant="success" className="ml-2">
                        {t('backup.verified')}
                      </Badge>
                    )}
                  </td>
                  <td className="py-1.5 pr-3 whitespace-nowrap">
                    {formatBytes(b.stored_bytes)}
                  </td>
                  <td className="py-1.5 pr-3">{b.mox_version}</td>
                  <td className="py-1.5 pr-3 break-all">{b.where}</td>
                  <td className="py-1.5 text-right">
                    <Reason why={why}>
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={!canWrite || running}
                        aria-label={t('backup.restoreFor', { id: b.id })}
                        onClick={() => setRestoreId(b.id)}
                      >
                        {t('backup.restore')}
                      </Button>
                    </Reason>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {restoreId && (
        <MailRestoreDialog
          app={app}
          backupId={restoreId}
          onClose={() => setRestoreId(null)}
        />
      )}
    </PanelCard>
  )
}
