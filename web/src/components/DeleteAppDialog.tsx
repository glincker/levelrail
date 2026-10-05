import { useState } from 'react'
import type { DialogControl } from './dialogControl'
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
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useDeleteApp } from '../queries/apps'

// One app's delete action, split out of the detail route the same way
// DeleteDatabaseDialog is split out of routes/databases/$name.tsx. DELETE
// /api/v1/apps/{name} (internal/api/apps.go's handleDeleteApp) is
// destructive and irreversible, so this is a confirm dialog. A 202 (app
// deleted, container cleanup still retrying) shows a warning toast. On
// success, navigates back to /apps.
export function DeleteAppDialog({
  name,
  control,
}: {
  name: string
  control?: DialogControl
}) {
  const [internalOpen, setInternalOpen] = useState(false)
  const open = control?.open ?? internalOpen
  const setOpen = (next: boolean) => {
    setInternalOpen(next)
    control?.onOpenChange?.(next)
  }
  const navigate = useNavigate()
  const { t } = useTranslation('common')
  const deleteApp = useDeleteApp()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      deleteApp.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      {control?.hideTrigger ? null : (
        <DialogTrigger render={<Button variant="destructive" size="sm" />}>
          <TrashIcon className="size-3.5" aria-hidden="true" />
          {t('deleteApp.trigger')}
        </DialogTrigger>
      )}
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5 text-destructive">
            <WarningIcon className="size-4" aria-hidden="true" />
            {t('deleteApp.title', { name })}
          </DialogTitle>
          <DialogDescription>{t('deleteApp.description')}</DialogDescription>
        </DialogHeader>
        {deleteApp.isError ? (
          <Alert variant="destructive">
            <WarningIcon />
            <AlertDescription>{deleteApp.error.message}</AlertDescription>
          </Alert>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              handleOpenChange(false)
            }}
          >
            {t('deleteApp.cancel')}
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={deleteApp.isPending}
            onClick={() => {
              deleteApp.mutate(name, {
                onSuccess: (result) => {
                  setOpen(false)
                  toast.add(
                    result.teardownPending
                      ? {
                          title: t('deleteApp.pendingTitle', { name }),
                          description: t('deleteApp.pendingDescription', {
                            reason: result.error,
                          }),
                          type: 'warning',
                        }
                      : {
                          title: t('deleteApp.deleted', { name }),
                          type: 'success',
                        },
                  )
                  void navigate({ to: '/apps' })
                },
              })
            }}
          >
            {deleteApp.isPending
              ? t('deleteApp.deleting')
              : t('deleteApp.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
