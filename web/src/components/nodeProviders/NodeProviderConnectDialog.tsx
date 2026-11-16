import { useState, type RefObject } from 'react'
import { useTranslation } from 'react-i18next'
import { UrlDialog } from '../kit/UrlDialog'
import { ManualEnrollFields } from '../AddNodeDialog'
import type { UrlDialogControls } from '../kit/useUrlDialog'
import type { NodeProviderResource } from '../../types/nodeProvision'
import { CONNECT_DIALOG_PARAMS } from './connectDialogParams'
import { NodeProviderForm } from './NodeProviderForm'
import { OracleCredentialForm } from './OracleCredentialForm'

// One credential dialog for the Cloud providers list and the provider landing page.
export function NodeProviderConnectDialog({
  provider,
  label,
  dialog,
  returnFocusRef,
}: {
  provider: NodeProviderResource
  label: string
  dialog: UrlDialogControls
  returnFocusRef: RefObject<HTMLButtonElement | null>
}) {
  const { t } = useTranslation(['settings', 'nodes'])
  const [addingNode, setAddingNode] = useState(false)
  return (
    <UrlDialog
      value={provider.provider}
      ownedParams={CONNECT_DIALOG_PARAMS}
      returnFocusRef={returnFocusRef}
      title={
        addingNode
          ? t('nodeProviders.addNodeTitle', { provider: label })
          : t('nodeProviders.sheetTitle', { provider: label })
      }
      description={
        addingNode
          ? t('nodeProviders.addNodeDescription')
          : t('nodeProviders.sheetDescription')
      }
    >
      {addingNode ? (
        <div className="space-y-3">
          <p className="rounded-lg border border-border bg-muted/30 p-3 text-xs text-muted-foreground">
            {t('nodeProviders.firewallGuidance')}
          </p>
          <ManualEnrollFields
            onDone={() => setAddingNode(false)}
            onBack={() => setAddingNode(false)}
          />
        </div>
      ) : provider.provider === 'oracle' ? (
        <OracleCredentialForm
          provider={provider}
          label={label}
          onSaved={dialog.close}
        />
      ) : (
        <NodeProviderForm
          provider={provider}
          label={label}
          onSaved={dialog.close}
          onAddNode={
            provider.provider === 'custom'
              ? () => setAddingNode(true)
              : undefined
          }
        />
      )}
    </UrlDialog>
  )
}
