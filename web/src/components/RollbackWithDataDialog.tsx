import { useTranslation } from 'react-i18next'
import {
  DatabaseGlassIcon,
  RotateCcwGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { RelativeTime } from './kit'
import {
  usePredeploySnapshots,
  useRollbackWithData,
} from '../queries/predeploySnapshot'

// Mirrors the server's usable-snapshot filter so the button is disabled before the 409.
function usableSnapshots<
  T extends { status: string; backup_id?: string; restore_status?: string },
>(rows: T[]): T[] {
  return rows.filter(
    (p) =>
      p.status === 'succeeded' &&
      Boolean(p.backup_id) &&
      p.restore_status !== 'running',
  )
}

export function RollbackWithDataDialog({
  appName,
  deployId,
  protectedEnv,
  open,
  onOpenChange,
}: {
  appName: string
  deployId: string
  protectedEnv: boolean
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation('deploys')
  const snapshots = usePredeploySnapshots(appName, open)
  const rollback = useRollbackWithData(appName)

  const forDeploy = (snapshots.data ?? []).filter(
    (p) => p.deploy_id === deployId,
  )
  const usable = usableSnapshots(forDeploy)

  let blockedReason: string | null = null
  if (protectedEnv) blockedReason = t('rollbackWithData.reasonProtected')
  else if (snapshots.isPending) blockedReason = t('rollbackWithData.loading')
  else if (snapshots.isError)
    blockedReason = t('rollbackWithData.reasonLoadFailed')
  else if (usable.length === 0)
    blockedReason = t('rollbackWithData.reasonNoSnapshot')

  function handleOpenChange(next: boolean) {
    onOpenChange(next)
    if (!next) rollback.reset()
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5">
            <DatabaseGlassIcon className="size-5" aria-hidden="true" />
            {t('rollbackWithData.title')}
          </DialogTitle>
          <DialogDescription>
            {t('rollbackWithData.description')}
          </DialogDescription>
        </DialogHeader>
        {usable.length > 0 ? (
          <ul
            aria-label={t('rollbackWithData.snapshotsLabel')}
            className="divide-y divide-border rounded-md border border-border text-sm"
          >
            {usable.map((p) => (
              <li
                key={p.id}
                className="flex items-center justify-between gap-3 px-3 py-2"
              >
                <span className="min-w-0 truncate font-medium text-foreground">
                  {p.database_name}
                </span>
                <span className="shrink-0 text-muted-foreground">
                  {t('rollbackWithData.takenAt')}{' '}
                  <RelativeTime at={p.created_at} />
                </span>
              </li>
            ))}
          </ul>
        ) : null}
        <Alert variant="destructive">
          <TriangleAlertGlassIcon className="text-tone-warning" size="1.4em" />
          <AlertDescription>{t('rollbackWithData.warning')}</AlertDescription>
        </Alert>
        {blockedReason ? (
          <p role="status" className="text-sm text-muted-foreground">
            {blockedReason}
          </p>
        ) : null}
        {rollback.isError ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.4em"
            />
            <AlertDescription>{rollback.error.message}</AlertDescription>
          </Alert>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => handleOpenChange(false)}
          >
            {t('rollbackWithData.cancel')}
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={blockedReason !== null || rollback.isPending}
            onClick={() => {
              rollback.mutate(deployId, {
                onSuccess: () => {
                  handleOpenChange(false)
                  toast.add({
                    title: t('rollbackWithData.started'),
                    type: 'success',
                  })
                },
              })
            }}
          >
            <RotateCcwGlassIcon className="size-5" aria-hidden="true" />
            {rollback.isPending
              ? t('rollbackWithData.working')
              : t('rollbackWithData.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
