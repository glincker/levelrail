import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { TrashIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { ApiError } from '@/lib/apiError'
import { useDeleteDatabase } from '../queries/databases'

const CONFLICT_STATUS = 409

// Confirm dialog for DELETE /api/v1/databases/{name}. A 409 means apps still
// connect to it, so the same button turns into an explicit "delete anyway".
export function DeleteDatabaseDialog({ name }: { name: string }) {
  const { t } = useTranslation('databases')
  const [open, setOpen] = useState(false)
  const navigate = useNavigate()
  const deleteDatabase = useDeleteDatabase()
  const inUse =
    deleteDatabase.error instanceof ApiError &&
    deleteDatabase.error.status === CONFLICT_STATUS

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      deleteDatabase.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="destructive" size="sm" />}>
        <TrashIcon className="size-3.5" aria-hidden="true" />
        {t('delete.trigger')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5 text-destructive">
            <WarningIcon className="size-4" aria-hidden="true" />
            {t('delete.title', { name })}
          </DialogTitle>
          <DialogDescription>{t('delete.description')}</DialogDescription>
        </DialogHeader>
        {deleteDatabase.isError ? (
          <div className="space-y-1 text-sm text-destructive" role="alert">
            {inUse ? (
              <p className="font-medium">{t('delete.inUseTitle')}</p>
            ) : null}
            <p>{deleteDatabase.error.message}</p>
            {inUse ? (
              <p className="text-muted-foreground">
                {t('delete.inUseDescription')}
              </p>
            ) : null}
          </div>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              handleOpenChange(false)
            }}
          >
            {t('delete.cancel')}
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={deleteDatabase.isPending}
            onClick={() => {
              deleteDatabase.mutate(
                { name, force: inUse },
                {
                  onSuccess: () => {
                    setOpen(false)
                    toast.add({
                      title: t('delete.toast', { name }),
                      type: 'success',
                    })
                    void navigate({ to: '/databases' })
                  },
                },
              )
            }}
          >
            {deleteDatabase.isPending
              ? t('delete.deleting')
              : inUse
                ? t('delete.confirmAnyway')
                : t('delete.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
