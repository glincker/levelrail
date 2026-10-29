import { useTranslation } from 'react-i18next'
import { TrashGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmAction } from '@/components/ui/confirm-action'
import { toast } from '@/components/ui/toast'
import { useDeleteRegistryCredential } from '../queries/registryCredentials'
import type { RegistryCredential } from '../types/registryCredential'

export function DeleteRegistryCredentialDialog({
  credential,
}: {
  credential: RegistryCredential
}) {
  const { t } = useTranslation('common')
  const deleteCredential = useDeleteRegistryCredential()

  return (
    <ConfirmAction
      trigger={(openDialog) => (
        <Button variant="destructive" size="sm" onClick={openDialog}>
          <TrashGlassIcon className="size-5" aria-hidden="true" />
          {t('confirmAction.registryCredential.trigger', {
            defaultValue: 'Delete',
          })}
        </Button>
      )}
      title={t('confirmAction.registryCredential.title', {
        defaultValue: 'Delete "{{name}}"?',
        name: credential.name,
      })}
      consequences={[
        t('confirmAction.registryCredential.consequence0', {
          defaultValue:
            'Any service referencing this credential fails to pull a private image on its next deploy.',
        }),
        t('confirmAction.registryCredential.consequence1', {
          defaultValue: 'This cannot be undone.',
        }),
      ]}
      confirmLabel={t('confirmAction.registryCredential.confirm', {
        defaultValue: 'Delete',
      })}
      pendingLabel={t('confirmAction.registryCredential.pending', {
        defaultValue: 'Deleting...',
      })}
      pending={deleteCredential.isPending}
      error={deleteCredential.isError ? deleteCredential.error.message : null}
      onOpenChange={(next) => {
        if (!next) deleteCredential.reset()
      }}
      onConfirm={(close) => {
        deleteCredential.mutate(credential.id, {
          onSuccess: () => {
            close()
            toast.add({
              title: t('confirmAction.registryCredential.success', {
                defaultValue: 'Registry credential "{{name}}" deleted.',
                name: credential.name,
              }),
              type: 'success',
            })
          },
          onError: (error) => {
            toast.add({
              title: t('confirmAction.registryCredential.error', {
                defaultValue: 'Could not delete "{{name}}".',
                name: credential.name,
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
