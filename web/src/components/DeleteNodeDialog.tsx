import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TrashGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import { useDeleteNode } from '../queries/nodes'
import type { DialogControl } from './dialogControl'

// DELETE /api/v1/nodes/{id} removes the registry row only: it neither
// drains the node nor disconnects a running agent, so the copy says so.
export function DeleteNodeDialog({
  id,
  name,
  control,
  onDeleted,
}: {
  id: string
  name: string
  control?: DialogControl
  onDeleted?: () => void
}) {
  const { t } = useTranslation('nodes')
  const [internalOpen, setInternalOpen] = useState(false)
  const open = control?.open ?? internalOpen
  const setOpen = (next: boolean) => {
    setInternalOpen(next)
    control?.onOpenChange?.(next)
  }
  const deleteNode = useDeleteNode()

  return (
    <>
      {control?.hideTrigger ? null : (
        <Button
          type="button"
          variant="destructive"
          size="sm"
          onClick={() => {
            setOpen(true)
          }}
        >
          <TrashGlassIcon className="size-5" aria-hidden="true" />
          {t('delete.trigger')}
        </Button>
      )}
      <ConfirmDialog
        open={open}
        onOpenChange={(next) => {
          setOpen(next)
          if (!next) {
            deleteNode.reset()
          }
        }}
        tone="destructive"
        title={t('delete.title', { name })}
        description={t('delete.description')}
        requireTyped={name}
        confirmLabel={t('delete.confirm')}
        pendingLabel={t('delete.deleting')}
        pending={deleteNode.isPending}
        error={deleteNode.isError ? deleteNode.error.message : null}
        onConfirm={() => {
          deleteNode.mutate(id, {
            onSuccess: () => {
              setOpen(false)
              onDeleted?.()
              toast.add({
                title: t('delete.deleted', { name }),
                type: 'success',
              })
            },
          })
        }}
      />
    </>
  )
}
