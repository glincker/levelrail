import { useTranslation } from 'react-i18next'
import { TrashGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmAction } from '@/components/ui/confirm-action'
import { toast } from '@/components/ui/toast'
import { useDeleteNetworkShare } from '../queries/networkShares'
import type { NetworkShare } from '../types/networkShare'

export function DeleteNetworkShareDialog({ share }: { share: NetworkShare }) {
  const { t } = useTranslation('common')
  const deleteShare = useDeleteNetworkShare()

  return (
    <ConfirmAction
      trigger={(openDialog) => (
        <Button variant="destructive" size="sm" onClick={openDialog}>
          <TrashGlassIcon className="size-4" aria-hidden="true" />
          {t('confirmAction.networkShare.trigger', { defaultValue: 'Delete' })}
        </Button>
      )}
      title={t('confirmAction.networkShare.title', {
        defaultValue: 'Delete "{{name}}"?',
        name: share.name,
      })}
      consequences={[
        t('confirmAction.networkShare.consequence0', {
          defaultValue:
            'Any app volume still mounting this share fails to start on its next deploy.',
        }),
        t('confirmAction.networkShare.consequence1', {
          defaultValue: 'This cannot be undone.',
        }),
      ]}
      confirmLabel={t('confirmAction.networkShare.confirm', {
        defaultValue: 'Delete',
      })}
      pendingLabel={t('confirmAction.networkShare.pending', {
        defaultValue: 'Deleting...',
      })}
      pending={deleteShare.isPending}
      error={deleteShare.isError ? deleteShare.error.message : null}
      onOpenChange={(next) => {
        if (!next) deleteShare.reset()
      }}
      onConfirm={(close) => {
        deleteShare.mutate(share.id, {
          onSuccess: () => {
            close()
            toast.add({
              title: t('confirmAction.networkShare.success', {
                defaultValue: 'Network share "{{name}}" deleted.',
                name: share.name,
              }),
              type: 'success',
            })
          },
          onError: (error) => {
            toast.add({
              title: t('confirmAction.networkShare.error', {
                defaultValue: 'Could not delete "{{name}}".',
                name: share.name,
              }),
              description: error.message,
              type: 'error',
            })
          },
        })
      }}
    />
  )
}
