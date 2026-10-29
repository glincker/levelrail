import { useTranslation } from 'react-i18next'
import { TrashGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmAction } from '@/components/ui/confirm-action'
import { toast } from '@/components/ui/toast'
import { useDeleteBackupTarget } from '../queries/backupTargets'
import type { BackupTarget } from '../types/backupTarget'

export function DeleteBackupTargetDialog({ target }: { target: BackupTarget }) {
  const { t } = useTranslation('common')
  const deleteBackupTarget = useDeleteBackupTarget()

  return (
    <ConfirmAction
      trigger={(openDialog) => (
        <Button variant="destructive" size="sm" onClick={openDialog}>
          <TrashGlassIcon className="size-5" aria-hidden="true" />
          {t('confirmAction.backupTarget.trigger', { defaultValue: 'Delete' })}
        </Button>
      )}
      title={t('confirmAction.backupTarget.title', {
        defaultValue: 'Delete "{{name}}"?',
        name: target.name,
      })}
      consequences={[
        t('confirmAction.backupTarget.consequence0', {
          defaultValue: 'The connection to this bucket is removed.',
        }),
        t('confirmAction.backupTarget.consequence1', {
          defaultValue: 'Nothing already backed up there is deleted.',
        }),
        t('confirmAction.backupTarget.consequence2', {
          defaultValue: 'This cannot be undone.',
        }),
      ]}
      confirmLabel={t('confirmAction.backupTarget.confirm', {
        defaultValue: 'Delete',
      })}
      pendingLabel={t('confirmAction.backupTarget.pending', {
        defaultValue: 'Deleting...',
      })}
      pending={deleteBackupTarget.isPending}
      error={
        deleteBackupTarget.isError ? deleteBackupTarget.error.message : null
      }
      onOpenChange={(next) => {
        if (!next) deleteBackupTarget.reset()
      }}
      onConfirm={(close) => {
        deleteBackupTarget.mutate(target.id, {
          onSuccess: () => {
            close()
            toast.add({
              title: t('confirmAction.backupTarget.success', {
                defaultValue: 'Backup target "{{name}}" deleted.',
                name: target.name,
              }),
              type: 'success',
            })
          },
          onError: (error) => {
            toast.add({
              title: t('confirmAction.backupTarget.error', {
                defaultValue: 'Could not delete "{{name}}".',
                name: target.name,
              }),
              description: error.message,
              type: 'error',
            })
          },
        })
      }}
    />
  )
}
