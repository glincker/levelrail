import { useState } from 'react'
import type { DialogControl } from './dialogControl'
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
import { useDeleteCustomTemplate } from '../queries/customTemplates'

// One custom template's delete action (DELETE
// /api/v1/templates/custom/{id}), the "Your templates" counterpart to
// DeleteAppDialog: destructive and irreversible (there is no undo), so
// a confirm dialog, not a bare one-click button in the marketplace
// grid. Deleting a template never touches the app it was saved from.
export function DeleteCustomTemplateDialog({
  id,
  name,
  control,
}: {
  id: string
  name: string
  control?: DialogControl
}) {
  const [internalOpen, setInternalOpen] = useState(false)
  const open = control?.open ?? internalOpen
  const setOpen = (next: boolean) => {
    setInternalOpen(next)
    control?.onOpenChange?.(next)
  }
  const deleteTemplate = useDeleteCustomTemplate()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      deleteTemplate.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      {control?.hideTrigger ? null : (
        <DialogTrigger
          render={
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label={`Delete template "${name}"`}
            />
          }
        >
          <TrashIcon className="size-3.5" aria-hidden="true" />
        </DialogTrigger>
      )}
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5 text-destructive">
            <WarningIcon className="size-4" aria-hidden="true" />
            Delete &ldquo;{name}&rdquo;?
          </DialogTitle>
          <DialogDescription>
            Removes this template from the catalog. The app it was saved from,
            and any apps already deployed from it, are unaffected. This cannot
            be undone.
          </DialogDescription>
        </DialogHeader>
        {deleteTemplate.isError ? (
          <Alert variant="destructive">
            <WarningIcon />
            <AlertDescription>{deleteTemplate.error.message}</AlertDescription>
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
            Cancel
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={deleteTemplate.isPending}
            onClick={() => {
              deleteTemplate.mutate(id, {
                onSuccess: () => {
                  setOpen(false)
                  toast.add({
                    title: `Template "${name}" deleted.`,
                    type: 'success',
                  })
                },
              })
            }}
          >
            {deleteTemplate.isPending ? 'Deleting...' : 'Delete'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
