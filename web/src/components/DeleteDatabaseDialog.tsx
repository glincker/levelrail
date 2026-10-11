import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import { ApiError } from '@/lib/apiError'
import { useDeleteDatabase } from '../queries/databases'

const CONFLICT_STATUS = 409

// DELETE /api/v1/databases/{name}. The name must be typed. A 409 means apps
// still connect to it, so the same dialog offers an explicit "delete anyway".
export function DeleteDatabaseDialog({
  name,
  open,
  onOpenChange,
}: {
  name: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation('databases')
  const navigate = useNavigate()
  const deleteDatabase = useDeleteDatabase()
  const inUse =
    deleteDatabase.error instanceof ApiError &&
    deleteDatabase.error.status === CONFLICT_STATUS

  function handleOpenChange(next: boolean) {
    onOpenChange(next)
    if (!next) {
      deleteDatabase.reset()
    }
  }

  const error = deleteDatabase.isError
    ? inUse
      ? `${t('delete.inUseTitle')}. ${deleteDatabase.error.message} ${t('delete.inUseDescription')}`
      : deleteDatabase.error.message
    : null

  return (
    <ConfirmDialog
      open={open}
      onOpenChange={handleOpenChange}
      title={t('delete.title', { name })}
      description={t('delete.description')}
      consequences={[t('delete.typedHint')]}
      requireTyped={name}
      tone="destructive"
      confirmLabel={inUse ? t('delete.confirmAnyway') : t('delete.confirm')}
      pendingLabel={t('delete.deleting')}
      cancelLabel={t('delete.cancel')}
      pending={deleteDatabase.isPending}
      error={error}
      onConfirm={() => {
        deleteDatabase.mutate(
          { name, force: inUse },
          {
            onSuccess: () => {
              onOpenChange(false)
              toast.add({ title: t('delete.toast', { name }), type: 'success' })
              void navigate({ to: '/databases' })
            },
          },
        )
      }}
    />
  )
}
