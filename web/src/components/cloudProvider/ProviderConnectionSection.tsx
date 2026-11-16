import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PlugGlassIcon, SettingsGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import { StatusPill } from '../kit/StatusPill'
import { RelativeTime } from '../kit/RelativeTime'
import { useUrlDialog } from '../kit/useUrlDialog'
import { CONNECT_DIALOG_PARAMS } from '../nodeProviders/connectDialogParams'
import { NodeProviderConnectDialog } from '../nodeProviders/NodeProviderConnectDialog'
import { useDisconnectNodeProvider } from '../../queries/nodeProvision'
import type { SectionProps } from './sections'

export function ProviderConnectionSection({
  provider,
  resource,
  labels,
}: SectionProps) {
  const { t } = useTranslation('cloudProvider')
  const triggerRef = useRef<HTMLButtonElement>(null)
  const dialog = useUrlDialog(provider.id, 'connect', CONNECT_DIALOG_PARAMS)
  const disconnect = useDisconnectNodeProvider()
  const [confirming, setConfirming] = useState(false)
  const connected = resource.has_token
  const failed = resource.status === 'test_failed'
  const manual = provider.id === 'custom'

  return (
    <section
      aria-labelledby="cp-connection"
      className="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-lg border border-border p-3"
    >
      <h2 id="cp-connection" className="text-sm font-semibold">
        {t('connection.title')}
      </h2>
      <StatusPill
        tone={failed ? 'danger' : connected ? 'success' : 'neutral'}
        label={
          failed
            ? t('connection.testFailed')
            : connected
              ? t('connection.connected')
              : t('connection.notConnected')
        }
        size="sm"
        title={failed ? resource.last_test_detail : undefined}
      />
      <p className="min-w-0 flex-1 basis-56 truncate text-xs text-muted-foreground">
        {connected && resource.connected_by ? (
          <>
            {t('connection.connectedBy', { name: resource.connected_by })}
            {resource.last_tested_at ? (
              <>
                {' · '}
                {t('connection.lastTested')}{' '}
                <RelativeTime at={resource.last_tested_at} />
              </>
            ) : null}
          </>
        ) : !connected && !manual ? (
          t('connection.notConnectedHint', {
            name: labels.name,
            servers: labels.servers,
          })
        ) : null}
      </p>
      <div className="flex items-center gap-2">
        <Button
          ref={triggerRef}
          type="button"
          variant="outline"
          size="sm"
          onClick={() => dialog.open()}
        >
          <SettingsGlassIcon size="1.4em" aria-hidden="true" />
          {connected ? t('connection.update') : t('connection.connect')}
        </Button>
        {connected ? (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => setConfirming(true)}
          >
            <PlugGlassIcon size="1.4em" aria-hidden="true" />
            {t('connection.disconnect')}
          </Button>
        ) : null}
      </div>
      <NodeProviderConnectDialog
        provider={resource}
        label={labels.name}
        dialog={dialog}
        returnFocusRef={triggerRef}
      />
      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title={t('connection.disconnectTitle', { name: labels.name })}
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
                title: t('connection.disconnected', { name: labels.name }),
                type: 'success',
              })
            },
          })
        }
      />
    </section>
  )
}
