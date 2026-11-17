import { useMemo, useState, type MouseEvent, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import { useUrlDialog } from '../kit/useUrlDialog'
import { CONNECT_DIALOG_PARAMS } from '../nodeProviders/connectDialogParams'
import { NodeProviderConnectDialog } from '../nodeProviders/NodeProviderConnectDialog'
import { useDisconnectNodeProvider } from '../../queries/nodeProvision'
import type { CloudProviderPresentation } from '../../lib/cloudProviderBrand'
import type { NodeProviderResource } from '../../types/nodeProvision'

export interface ProviderConnection {
  /** Remembers the clicked button so focus returns to it when the dialog closes. */
  openConnect: (e: MouseEvent<HTMLButtonElement>) => void
  askDisconnect: () => void
  /** Mount once on the page so every Connect button shares one dialog. */
  dialogs: ReactNode
}

export function useProviderConnection(
  provider: CloudProviderPresentation,
  resource: NodeProviderResource,
  name: string,
): ProviderConnection {
  const { t } = useTranslation('cloudProvider')
  const [trigger, setTrigger] = useState<HTMLButtonElement | null>(null)
  const focusReturn = useMemo(() => ({ current: trigger }), [trigger])
  const dialog = useUrlDialog(provider.id, 'connect', CONNECT_DIALOG_PARAMS)
  const disconnect = useDisconnectNodeProvider()
  const [confirming, setConfirming] = useState(false)

  const dialogs = (
    <>
      <NodeProviderConnectDialog
        provider={resource}
        label={name}
        dialog={dialog}
        returnFocusRef={focusReturn}
      />
      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title={t('connection.disconnectTitle', { name })}
        description={t('connection.disconnectBody')}
        confirmLabel={t('connection.disconnectConfirm')}
        tone="destructive"
        pending={disconnect.isPending}
        error={disconnect.isError ? disconnect.error.message : null}
        onConfirm={() =>
          disconnect.mutate(provider.id, {
            onSuccess: () => {
              setConfirming(false)
              toast.add({
                title: t('connection.disconnected', { name }),
                type: 'success',
              })
            },
          })
        }
      />
    </>
  )
  return {
    openConnect: (e) => {
      setTrigger(e.currentTarget)
      dialog.open()
    },
    askDisconnect: () => setConfirming(true),
    dialogs,
  }
}
