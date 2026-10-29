import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SettingsGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { BrandTile } from '../BrandLogoBadge'
import { StatusPill } from '../kit/StatusPill'
import { UrlDialog } from '../kit/UrlDialog'
import { useUrlDialog } from '../kit/useUrlDialog'
import { RelativeTime } from '../kit/RelativeTime'
import { ManualEnrollFields } from '../AddNodeDialog'
import { nodeProviderBrand } from '../../lib/providerBrands'
import type { NodeProviderResource } from '../../types/nodeProvision'
import { NodeProviderForm } from './NodeProviderForm'

const NAME_KEYS = {
  hetzner: 'nodeProviders.names.hetzner',
  digitalocean: 'nodeProviders.names.digitalocean',
  aws: 'nodeProviders.names.aws',
  azure: 'nodeProviders.names.azure',
  gcp: 'nodeProviders.names.gcp',
  custom: 'nodeProviders.names.custom',
} as const

const HINT_KEYS = {
  hetzner: 'nodeProviders.hints.hetzner',
  digitalocean: 'nodeProviders.hints.digitalocean',
  aws: 'nodeProviders.hints.aws',
  azure: 'nodeProviders.hints.azure',
  gcp: 'nodeProviders.hints.gcp',
  custom: 'nodeProviders.hints.custom',
} as const

type KnownProvider = keyof typeof NAME_KEYS

function asKnown(name: string): KnownProvider | null {
  return name in NAME_KEYS ? (name as KnownProvider) : null
}

const DIALOG_PARAMS = ['method'] as const

export function NodeProviderRow({
  provider,
}: {
  provider: NodeProviderResource
}) {
  const { t } = useTranslation('settings')
  const triggerRef = useRef<HTMLButtonElement>(null)
  const dialog = useUrlDialog(provider.provider, 'connect', DIALOG_PARAMS)
  const [addingNode, setAddingNode] = useState(false)
  const known = asKnown(provider.provider)
  const label = known ? t(NAME_KEYS[known]) : provider.provider
  const connected = provider.has_token
  const failed = provider.status === 'test_failed'
  const detail =
    provider.label && provider.provider === 'custom'
      ? [provider.label, provider.location].filter(Boolean).join(', ')
      : known
        ? t(HINT_KEYS[known])
        : ''

  return (
    <li className="flex flex-wrap items-center gap-x-3 gap-y-2 py-3">
      <BrandTile
        name={nodeProviderBrand(provider.provider)}
        label={label}
        labelled
      />
      <div className="min-w-0 flex-1 basis-56">
        <p className="text-sm font-medium">{label}</p>
        <p className="truncate text-xs text-muted-foreground">{detail}</p>
        {connected && provider.connected_by ? (
          <p className="truncate text-xs text-muted-foreground">
            {t('nodeProviders.connectedBy', { name: provider.connected_by })}
            {provider.last_tested_at ? (
              <>
                {' · '}
                {t('nodeProviders.lastTested')}{' '}
                <RelativeTime at={provider.last_tested_at} />
              </>
            ) : null}
          </p>
        ) : null}
      </div>
      <StatusPill
        tone={failed ? 'danger' : connected ? 'success' : 'neutral'}
        label={
          failed
            ? t('nodeProviders.testFailed')
            : connected
              ? t('nodeProviders.connected')
              : t('nodeProviders.notConnected')
        }
        size="sm"
        title={failed ? provider.last_test_detail : undefined}
      />
      <Button
        ref={triggerRef}
        type="button"
        variant="outline"
        size="sm"
        aria-label={t('nodeProviders.configureFor', { provider: label })}
        onClick={() => {
          setAddingNode(false)
          dialog.open()
        }}
      >
        <SettingsGlassIcon size="1.4em" aria-hidden="true" />
        {connected ? t('nodeProviders.update') : t('nodeProviders.connect')}
      </Button>
      <UrlDialog
        value={provider.provider}
        ownedParams={DIALOG_PARAMS}
        returnFocusRef={triggerRef}
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
    </li>
  )
}
