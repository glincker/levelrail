import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { RefreshCwGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import { useRenewNodeCert } from '../../queries/nodeTrust'
import type { NodeResource } from '../../types/nodeDetail'

// Root only: asks the connected agent to renew now. The agent verifies the
// new certificate before it replaces the old one.
export function RenewNowDialog({ node }: { node: NodeResource }) {
  const { t } = useTranslation('nodes')
  const [open, setOpen] = useState(false)
  const renew = useRenewNodeCert()
  return (
    <>
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={node.cert?.state === 'revoked' || node.status !== 'online'}
        title={node.status !== 'online' ? t('trust.renewOffline') : undefined}
        onClick={() => {
          setOpen(true)
        }}
      >
        <RefreshCwGlassIcon className="size-4" aria-hidden="true" />
        {t('trust.renewNow')}
      </Button>
      <ConfirmDialog
        open={open}
        onOpenChange={(next) => {
          setOpen(next)
          if (!next) renew.reset()
        }}
        title={t('trust.renewTitle', { name: node.name })}
        description={t('trust.renewDescription')}
        consequences={[
          t('trust.renewConsequenceVerify'),
          t('trust.renewConsequenceKeep'),
          t('trust.renewConsequenceAudit'),
        ]}
        confirmLabel={t('trust.renewConfirm')}
        pendingLabel={t('trust.renewPending')}
        pending={renew.isPending}
        error={renew.isError ? renew.error.message : null}
        onConfirm={() => {
          renew.mutate(node.id, {
            onSuccess: () => {
              setOpen(false)
              toast.add({
                title: t('trust.renewRequested', { name: node.name }),
                type: 'success',
              })
            },
          })
        }}
      />
    </>
  )
}
