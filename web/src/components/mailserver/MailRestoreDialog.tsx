import { useCallback, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { formatBytes } from '@/lib/modelPreflight'
import { useStepUpGate } from '../../hooks/useStepUpGate'
import {
  mailBackupKeys,
  mailRestoreStepUpScope,
  restoreDryRun,
  restoreMailBackup,
} from '../../queries/mailBackup'
import { EnvApiError } from '../../queries/envProtection'
import { useInvalidateMail } from '../../queries/mailMailboxes'
import { StepUpDialog } from '../env/StepUpDialog'

/** Dry run first, then a typed id, step-up and the real restore. */
export function MailRestoreDialog({
  app,
  backupId,
  onClose,
}: {
  app: string
  backupId: string
  onClose: () => void
}) {
  const { t } = useTranslation('mailServer')
  const invalidate = useInvalidateMail()
  const [typed, setTyped] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const dry = useQuery({
    queryKey: [...mailBackupKeys.backups(app), 'dry', backupId],
    queryFn: () => restoreDryRun(app, backupId),
    retry: false,
    gcTime: 0,
  })
  const onError = useCallback(
    (e: unknown) => {
      setBusy(false)
      setError(
        e instanceof EnvApiError && e.status === 409
          ? t('restore.conflict')
          : e instanceof Error
            ? e.message
            : t('restore.failed'),
      )
    },
    [t],
  )
  const gate = useStepUpGate(onError)
  const run = async () => {
    await restoreMailBackup(app, backupId)
    await invalidate()
    onClose()
  }
  const submit = () => {
    setError(null)
    setBusy(true)
    void gate.guard(mailRestoreStepUpScope(app), run)
  }
  const info = dry.data
  const dryError = dry.error instanceof Error ? dry.error : null
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('restore.title', { id: backupId })}</DialogTitle>
          <DialogDescription>
            {dry.isPending
              ? t('restore.checking')
              : info
                ? t('restore.summary', {
                    files: info.files ?? 0,
                    size: formatBytes(info.bytes ?? 0),
                    mox: info.mox_version ?? '',
                  })
                : ''}
          </DialogDescription>
        </DialogHeader>
        {dryError && (
          <p role="alert" className="text-xs text-tone-danger">
            {dryError instanceof EnvApiError && dryError.status === 409
              ? t('restore.conflict')
              : dryError.message}
          </p>
        )}
        {info && (
          <div className="space-y-2">
            <p className="text-xs text-tone-warning">{t('restore.warning')}</p>
            <div className="flex flex-wrap items-center gap-2">
              <Label htmlFor="restore-confirm" className="text-xs">
                {t('restore.confirmLabel', { id: backupId })}
              </Label>
              <Input
                id="restore-confirm"
                value={typed}
                autoComplete="off"
                className="h-8 w-56 font-mono"
                onChange={(e) => setTyped(e.target.value)}
              />
            </div>
          </div>
        )}
        {error && (
          <p role="alert" className="text-xs text-tone-danger">
            {error}
          </p>
        )}
        <DialogFooter>
          <Button
            variant="destructive"
            disabled={!info || typed !== backupId || busy}
            onClick={submit}
          >
            {busy ? t('restore.working') : t('restore.submit')}
          </Button>
        </DialogFooter>
        <StepUpDialog
          scope={gate.pending?.scope ?? null}
          onVerified={() => {
            void gate.onVerified()
          }}
          onCancel={() => {
            setBusy(false)
            gate.cancel()
          }}
        />
      </DialogContent>
    </Dialog>
  )
}
