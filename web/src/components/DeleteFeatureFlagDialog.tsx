import { useTranslation } from 'react-i18next'
import { TrashGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmAction } from '@/components/ui/confirm-action'
import { toast } from '@/components/ui/toast'
import { useDeleteFeatureFlag } from '../queries/featureFlags'
import type { FeatureFlag } from '../types/featureFlags'

export function DeleteFeatureFlagDialog({
  appName,
  flag,
}: {
  appName: string
  flag: FeatureFlag
}) {
  const { t } = useTranslation('common')
  const deleteFlag = useDeleteFeatureFlag(appName)

  return (
    <ConfirmAction
      trigger={(openDialog) => (
        <Button variant="destructive" size="sm" onClick={openDialog}>
          <TrashGlassIcon className="size-4" aria-hidden="true" />
          {t('confirmAction.featureFlag.trigger', { defaultValue: 'Delete' })}
        </Button>
      )}
      title={t('confirmAction.featureFlag.title', {
        defaultValue: 'Delete "{{name}}"?',
        name: flag.key,
      })}
      consequences={[
        t('confirmAction.featureFlag.consequence0', {
          defaultValue: 'The flag stops resolving immediately.',
        }),
        t('confirmAction.featureFlag.consequence1', {
          defaultValue:
            'Any app still calling the evaluate endpoint for it starts getting a 404.',
        }),
        t('confirmAction.featureFlag.consequence2', {
          defaultValue: 'This cannot be undone.',
        }),
      ]}
      confirmLabel={t('confirmAction.featureFlag.confirm', {
        defaultValue: 'Delete flag',
      })}
      pendingLabel={t('confirmAction.featureFlag.pending', {
        defaultValue: 'Deleting...',
      })}
      pending={deleteFlag.isPending}
      error={deleteFlag.isError ? deleteFlag.error.message : null}
      onOpenChange={(next) => {
        if (!next) deleteFlag.reset()
      }}
      onConfirm={(close) => {
        deleteFlag.mutate(flag.id, {
          onSuccess: () => {
            close()
            toast.add({
              title: t('confirmAction.featureFlag.success', {
                defaultValue: 'Feature flag deleted.',
                name: flag.key,
              }),
              type: 'success',
            })
          },
        })
      }}
    />
  )
}
