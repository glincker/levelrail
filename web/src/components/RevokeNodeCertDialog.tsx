import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { BanGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import { useRevokeNodeCert } from '../queries/nodeCert'
import type { NodeResource } from '../types/nodeDetail'

export function RevokeNodeCertDialog({ node }: { node: NodeResource }) {
  const { t } = useTranslation('nodes')
  const [open, setOpen] = useState(false)
  const revoke = useRevokeNodeCert()
  return (
    <>
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={node.cert?.state === 'revoked'}
        onClick={() => {
          setOpen(true)
        }}
      >
        <BanGlassIcon className="size-5" aria-hidden="true" />
        {t('revoke.trigger')}
      </Button>
      <ConfirmDialog
        open={open}
        onOpenChange={(next) => {
          setOpen(next)
          if (!next) revoke.reset()
        }}
        title={t('revoke.title', { name: node.name })}
        description={t('revoke.description')}
        consequences={[
          t('revoke.consequenceRefused'),
          t('revoke.consequenceSession'),
          t('revoke.consequenceWorkloads'),
          t('revoke.consequenceReenroll'),
        ]}
        tone="destructive"
        confirmLabel={t('revoke.confirm')}
        pendingLabel={t('revoke.pending')}
        pending={revoke.isPending}
        error={revoke.isError ? revoke.error.message : null}
        onConfirm={() => {
          revoke.mutate(node.id, {
            onSuccess: () => {
              setOpen(false)
              toast.add({
                title: t('revoke.done', { name: node.name }),
                type: 'success',
              })
            },
          })
        }}
      />
    </>
  )
}
