import { useTranslation } from 'react-i18next'
import { TrashGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmAction } from '@/components/ui/confirm-action'
import { toast } from '@/components/ui/toast'
import { useDeleteScheduledTask } from '../queries/scheduledTasks'
import type { ScheduledTask } from '../types/scheduledTasks'

export function DeleteScheduledTaskDialog({
  appName,
  task,
}: {
  appName: string
  task: ScheduledTask
}) {
  const { t } = useTranslation('jobs')
  const deleteTask = useDeleteScheduledTask(appName)

  return (
    <ConfirmAction
      trigger={(openDialog) => (
        <Button variant="destructive" size="sm" onClick={openDialog}>
          <TrashGlassIcon className="size-4" aria-hidden="true" />
          {t('delete.trigger')}
        </Button>
      )}
      title={t('delete.title')}
      consequences={[t('delete.description')]}
      confirmLabel={t('delete.confirm')}
      pendingLabel={t('dialog.saving')}
      pending={deleteTask.isPending}
      error={deleteTask.isError ? deleteTask.error.message : null}
      onOpenChange={(next) => {
        if (!next) deleteTask.reset()
      }}
      onConfirm={(close) => {
        deleteTask.mutate(task.id, {
          onSuccess: () => {
            close()
            toast.add({ title: t('delete.deleted'), type: 'success' })
          },
        })
      }}
    />
  )
}
