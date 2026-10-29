import { useTranslation } from 'react-i18next'
import { TrashGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmAction } from '@/components/ui/confirm-action'
import { toast } from '@/components/ui/toast'
import { useDeleteModel } from '../queries/models'

export function DeleteModelDialog({
  name,
  disabled,
}: {
  name: string
  disabled?: boolean
}) {
  const { t } = useTranslation('common')
  const deleteModel = useDeleteModel()

  return (
    <ConfirmAction
      trigger={(openDialog) => (
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={`${t('confirmAction.model.trigger', { defaultValue: 'Delete' })} ${name}`}
          title={t('confirmAction.model.trigger', { defaultValue: 'Delete' })}
          disabled={disabled}
          onClick={openDialog}
        >
          <TrashGlassIcon size="1.4em" aria-hidden="true" />
        </Button>
      )}
      title={t('confirmAction.model.title', {
        defaultValue: 'Delete "{{name}}"?',
        name: name,
      })}
      consequences={[
        t('confirmAction.model.consequence0', {
          defaultValue:
            'The engine container is removed and the API key stops working.',
        }),
        t('confirmAction.model.consequence1', {
          defaultValue:
            'Downloaded weights are kept, so deploying the same model again does not download them twice.',
        }),
      ]}
      confirmLabel={t('confirmAction.model.confirm', {
        defaultValue: 'Delete',
      })}
      pendingLabel={t('confirmAction.model.pending', {
        defaultValue: 'Deleting...',
      })}
      pending={deleteModel.isPending}
      error={deleteModel.isError ? deleteModel.error.message : null}
      onOpenChange={(next) => {
        if (!next) deleteModel.reset()
      }}
      onConfirm={(close) => {
        deleteModel.mutate(name, {
          onSuccess: () => {
            close()
            toast.add({
              title: t('confirmAction.model.success', {
                defaultValue: 'Model "{{name}}" deleted.',
                name: name,
              }),
              type: 'success',
            })
          },
        })
      }}
    />
  )
}
