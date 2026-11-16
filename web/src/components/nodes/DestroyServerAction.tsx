import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { TrashGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import { ApiError } from '../../lib/apiError'
import {
  useDestroyNodeProvision,
  useNodeProvisions,
} from '../../queries/nodeProvision'

// Shown only for nodes this control plane created at a cloud provider, so a
// registered or SSH node never offers a destroy it cannot perform.
export function DestroyServerAction({ nodeId }: { nodeId: string }) {
  const { t } = useTranslation('nodes')
  const navigate = useNavigate()
  const provisions = useNodeProvisions()
  const destroy = useDestroyNodeProvision()
  const [open, setOpen] = useState(false)
  const [force, setForce] = useState(false)

  const provision = provisions.data?.find((p) => p.node_id === nodeId)
  if (!provision) return null

  if (provision.status === 'destroying') {
    return (
      <Badge variant="warning">{t('provisionDestroy.statusDestroying')}</Badge>
    )
  }
  if (provision.status === 'destroyed') {
    return (
      <Badge variant="muted">{t('provisionDestroy.statusDestroyed')}</Badge>
    )
  }

  const needsForce =
    destroy.error instanceof ApiError && destroy.error.status === 409
  const otherError =
    destroy.isError && !needsForce ? destroy.error.message : null

  const close = (next: boolean) => {
    setOpen(next)
    if (!next) {
      destroy.reset()
      setForce(false)
    }
  }

  return (
    <>
      <Button
        type="button"
        variant="destructive"
        size="sm"
        onClick={() => {
          setOpen(true)
        }}
      >
        <TrashGlassIcon className="size-5" aria-hidden="true" />
        {t('provisionDestroy.trigger')}
      </Button>
      <ConfirmDialog
        open={open}
        onOpenChange={close}
        tone="destructive"
        title={t('provisionDestroy.title', { name: provision.name })}
        description={t('provisionDestroy.description', {
          provider: provision.provider,
        })}
        requireTyped={provision.name}
        confirmLabel={t('provisionDestroy.confirm')}
        pendingLabel={t('provisionDestroy.destroying')}
        pending={destroy.isPending}
        confirmDisabled={needsForce && !force}
        error={otherError}
        onConfirm={() => {
          destroy.mutate(
            { id: provision.id, force: needsForce && force },
            {
              onSuccess: () => {
                close(false)
                toast.add({
                  title: t('provisionDestroy.destroyed', {
                    name: provision.name,
                  }),
                  type: 'success',
                })
                void navigate({ to: '/nodes' })
              },
            },
          )
        }}
      >
        {needsForce ? (
          <div className="space-y-2">
            <Alert variant="destructive">
              <TriangleAlertGlassIcon
                className="text-tone-warning"
                size="1.7em"
              />
              <AlertTitle>{t('provisionDestroy.workloadsTitle')}</AlertTitle>
              <AlertDescription>
                {t('provisionDestroy.workloadsBody')}
              </AlertDescription>
            </Alert>
            <div className="flex items-center gap-2">
              <Checkbox
                id="provision-destroy-force"
                checked={force}
                onCheckedChange={(v) => {
                  setForce(v === true)
                }}
              />
              <Label htmlFor="provision-destroy-force" className="font-normal">
                {t('provisionDestroy.force')}
              </Label>
            </div>
          </div>
        ) : null}
      </ConfirmDialog>
    </>
  )
}
