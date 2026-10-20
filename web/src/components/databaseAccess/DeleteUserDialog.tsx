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
import { toast } from '@/components/ui/toast'
import { useDeleteDatabaseUser } from '../../queries/databaseAccess'

/** DeleteUserDialog confirms before dropping a login; name is null while closed. */
export function DeleteUserDialog({
  databaseName,
  name,
  onClose,
}: {
  databaseName: string
  name: string | null
  onClose: () => void
}) {
  const { t } = useTranslation('databaseAccess')
  const del = useDeleteDatabaseUser(databaseName)

  function confirm() {
    if (!name) return
    del.mutate(name, {
      onSuccess: () => {
        toast.add({ title: t('users.deleteToast', { name }), type: 'success' })
        onClose()
      },
      onError: (err) =>
        toast.add({
          title: t('users.failedToast'),
          description: err.message,
          type: 'error',
        }),
    })
  }

  return (
    <Dialog
      open={name !== null}
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {t('users.deleteTitle', { name: name ?? '' })}
          </DialogTitle>
          <DialogDescription>{t('users.deleteBody')}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t('users.cancel')}
          </Button>
          <Button
            variant="destructive"
            disabled={del.isPending}
            onClick={confirm}
          >
            {t('users.deleteConfirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
