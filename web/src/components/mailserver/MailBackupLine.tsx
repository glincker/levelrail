import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArchiveGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import { formatAge } from '@/lib/format'
import { formatBytes } from '@/lib/modelPreflight'
import { useCanWriteSensitive } from '../../hooks/useCanWriteSensitive'
import {
  useMailBackups,
  useMailStorage,
  useRunMailBackup,
  type MailBackupJob,
} from '../../queries/mailBackup'
import { MailBackupHistorySheet } from './MailBackupHistorySheet'
import { MailRestoreDialog } from './MailRestoreDialog'
import { Reason } from './mailPanelUi'

// Steps a backup job walks through, for the inline progress bar.
const BACKUP_STEPS = ['version', 'backup', 'verify', 'copy', 'archive', 'prune']

interface Props {
  app: string
  onStorage: () => void
}

/** Backups as one status line: last result, a primary run button and the destination chip. */
export function MailBackupLine({ app, onStorage }: Props) {
  const { t } = useTranslation('mailServer')
  const q = useMailBackups(app)
  const storage = useMailStorage(app).data
  const canWrite = useCanWriteSensitive()
  const run = useRunMailBackup(app)
  const [history, setHistory] = useState(false)
  const [restoreId, setRestoreId] = useState<string | null>(null)
  if (!q.data) return null
  const { backups, job, encrypted, cap_bytes } = q.data
  const running = job?.state === 'running'
  const last = backups[0]
  const why = canWrite ? undefined : t('backup.needsWrite')
  const step = (j: MailBackupJob) =>
    t(`backup.steps.${j.step}`, { defaultValue: j.step })
  const idx = job ? BACKUP_STEPS.indexOf(job.step) : -1
  const pct = idx >= 0 ? ((idx + 1) / BACKUP_STEPS.length) * 100 : 10
  const destination = [
    storage ? t(storage.kind === 's3' ? 'storage.s3' : 'storage.local') : null,
    cap_bytes > 0 ? t('backup.capChip', { cap: formatBytes(cap_bytes) }) : null,
  ]
    .filter(Boolean)
    .join(', ')

  return (
    <section
      aria-label={t('backup.title')}
      className="space-y-2 rounded-lg border border-border bg-card p-3"
    >
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2 text-sm">
        <ArchiveGlassIcon size="1.3em" />
        <p className="min-w-0 flex-1" role="status">
          {running && job
            ? job.kind === 'restore'
              ? t('backup.restoring', { step: step(job) })
              : t('backup.running', { step: step(job) })
            : last
              ? t('backup.line', {
                  when: formatAge(last.created_at),
                  size: formatBytes(last.stored_bytes),
                  state: t(
                    last.verified
                      ? 'backup.verifiedLower'
                      : 'backup.unverifiedLower',
                  ),
                })
              : t('backup.never')}
        </p>
        <button
          type="button"
          onClick={onStorage}
          title={t('backup.destinationOpen')}
          className="rounded-full border border-border px-2.5 py-0.5 text-xs hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring"
        >
          {destination || t('backup.destination')}
        </button>
        <Button size="sm" variant="ghost" onClick={() => setHistory(true)}>
          {t('backup.history')}
        </Button>
        <Reason why={why}>
          <Button
            size="sm"
            disabled={!canWrite || running || run.isPending || !encrypted}
            onClick={() => run.mutate()}
          >
            {t('backup.runNow')}
          </Button>
        </Reason>
      </div>
      {running ? (
        <Progress value={pct} aria-label={t('backup.progressLabel')} />
      ) : null}
      {!encrypted && (
        <p className="text-xs text-tone-warning">{t('backup.notEncrypted')}</p>
      )}
      {job?.state === 'failed' && (
        <p role="alert" className="text-xs text-tone-danger">
          {t('backup.jobFailed', { error: job.error ?? '' })}
        </p>
      )}
      {run.isError && (
        <p role="alert" className="text-xs text-tone-danger">
          {run.error instanceof Error ? run.error.message : t('backup.failed')}
        </p>
      )}
      <MailBackupHistorySheet
        open={history}
        onOpenChange={setHistory}
        data={q.data}
        canWrite={canWrite}
        onRestore={(id) => {
          setHistory(false)
          setRestoreId(id)
        }}
      />
      {restoreId && (
        <MailRestoreDialog
          app={app}
          backupId={restoreId}
          onClose={() => setRestoreId(null)}
        />
      )}
    </section>
  )
}
