import { useState } from 'react'
import type { DialogControl } from './dialogControl'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { TrashGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import { useDeleteApp } from '../queries/apps'
import { DeleteDataOption } from './backups/DeleteDataOption'

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
  const [deleteData, setDeleteData] = useState(false)

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      deleteApp.reset()
      setDeleteData(false)
    }
  }

  return (
    <>
      {control?.hideTrigger ? null : (
        <Button
          variant="destructive"
          size="sm"
          onClick={() => {
            setOpen(true)
          }}
        >
          <TrashGlassIcon className="size-5" aria-hidden="true" />
          {t('deleteApp.trigger')}
        </Button>
      )}
      <ConfirmDialog
        open={open}
        onOpenChange={handleOpenChange}
        tone="destructive"
        title={t('deleteApp.title', { name })}
        description={t('deleteApp.description')}
        requireTyped={name}
        confirmLabel={t('deleteApp.confirm', { name })}
        pendingLabel={t('deleteApp.deleting')}
        pending={deleteApp.isPending}
        error={deleteApp.isError ? deleteApp.error.message : null}
        onConfirm={() => {
          deleteApp.mutate(
            { name, deleteData },
            {
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
            },
          )
        }}
      >
        <DeleteDataOption
          kind="app"
          checked={deleteData}
          onCheckedChange={setDeleteData}
        />
      </ConfirmDialog>
    </>
  )
}
