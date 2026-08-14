import { useState } from 'react'
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
import { useDeleteAlertRule } from '../queries/alerts'
import type { AlertRule } from '../types/alerts'

// One rule's delete action, split out of AlertRulesPanel the same way
// RevokeTokenDialog is split out of TokenTable. DELETE
// /api/v1/apps/{name}/alerts/{id} (internal/api/alerts.go's
// handleDeleteAlertRule) is destructive and irreversible, there is no
// undo, so this is a confirm dialog, not a bare one-click button, same
// reasoning RevokeTokenDialog's own header comment gives.
export function DeleteAlertRuleDialog({
  appName,
  rule,
}: {
  appName: string
  rule: AlertRule
}) {
  const [open, setOpen] = useState(false)
  const deleteRule = useDeleteAlertRule(appName)

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      deleteRule.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="destructive" size="sm" />}>
        <TrashIcon className="size-3.5" aria-hidden="true" />
        Delete
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5 text-destructive">
            <WarningIcon className="size-4" aria-hidden="true" />
            Delete &ldquo;{rule.name}&rdquo;?
          </DialogTitle>
          <DialogDescription>
            This rule stops evaluating and notifying immediately. This cannot be
            undone.
          </DialogDescription>
        </DialogHeader>
        {deleteRule.isError ? (
          <p className="text-sm text-destructive">{deleteRule.error.message}</p>
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
            disabled={deleteRule.isPending}
            onClick={() => {
              deleteRule.mutate(rule.id, {
                onSuccess: () => {
                  setOpen(false)
                  toast.add({
                    title: `Alert rule "${rule.name}" deleted.`,
                    type: 'success',
                  })
                },
              })
            }}
          >
            {deleteRule.isPending ? 'Deleting...' : 'Delete rule'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
