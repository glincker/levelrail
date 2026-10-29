import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { LockGlassIcon, LockOpenGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import { useCordonNode, useUncordonNode } from '../queries/nodes'
import type { NodeResource } from '../types/nodeDetail'
import type { DialogControl } from './dialogControl'

// Cordon and uncordon are both idempotent POSTs, so a double click or a
// node already in the target state is harmless.
export function CordonNodeDialog({
  node,
  control,
}: {
  node: NodeResource
  control?: DialogControl
}) {
  const { t } = useTranslation('nodes')
  const [internalOpen, setInternalOpen] = useState(false)
  const open = control?.open ?? internalOpen
  const setOpen = (next: boolean) => {
    setInternalOpen(next)
    control?.onOpenChange?.(next)
  }
  const cordon = useCordonNode()
  const uncordon = useUncordonNode()
  const mutation = node.schedulable ? cordon : uncordon
  const key = node.schedulable ? 'cordon' : 'uncordon'
  const TriggerIcon = node.schedulable ? LockGlassIcon : LockOpenGlassIcon

  return (
    <>
      {control?.hideTrigger ? null : (
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => {
            setOpen(true)
          }}
        >
          <TriggerIcon className="size-3.5" aria-hidden="true" />
          {t(`${key}.trigger`)}
        </Button>
      )}
      <ConfirmDialog
        open={open}
        onOpenChange={(next) => {
          setOpen(next)
          if (!next) {
            cordon.reset()
            uncordon.reset()
          }
        }}
        title={t(`${key}.title`, { name: node.name })}
        description={t(`${key}.description`)}
        confirmLabel={t(`${key}.confirm`)}
        pendingLabel={t('cordon.saving')}
        pending={mutation.isPending}
        error={mutation.isError ? mutation.error.message : null}
        onConfirm={() => {
          mutation.mutate(node.id, {
            onSuccess: () => {
              setOpen(false)
              toast.add({
                title: t(`${key}.done`, { name: node.name }),
                type: 'success',
              })
            },
          })
        }}
      />
    </>
  )
}
