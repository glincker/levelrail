import { useRef } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { SettingsGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { BrandTile } from '../BrandLogoBadge'
import { StatusPill } from '../kit/StatusPill'
import { useUrlDialog } from '../kit/useUrlDialog'
import { ProviderRowStatus } from '../cloudProvider/ProviderRowStatus'
import { RelativeTime } from '../kit/RelativeTime'
import { nodeProviderBrand } from '../../lib/providerBrands'
import type { NodeProviderResource } from '../../types/nodeProvision'
import { CONNECT_DIALOG_PARAMS } from './connectDialogParams'
import { NodeProviderAccounts } from './NodeProviderAccounts'
import { NodeProviderConnectDialog } from './NodeProviderConnectDialog'

const NAME_KEYS = {
  hetzner: 'nodeProviders.names.hetzner',
  digitalocean: 'nodeProviders.names.digitalocean',
  vultr: 'nodeProviders.names.vultr',
  aws: 'nodeProviders.names.aws',
  azure: 'nodeProviders.names.azure',
  gcp: 'nodeProviders.names.gcp',
  oracle: 'nodes:oracle.name',
  custom: 'nodeProviders.names.custom',
} as const

const HINT_KEYS = {
  hetzner: 'nodeProviders.hints.hetzner',
  digitalocean: 'nodeProviders.hints.digitalocean',
  vultr: 'nodeProviders.hints.vultr',
  aws: 'nodeProviders.hints.aws',
  azure: 'nodeProviders.hints.azure',
  gcp: 'nodeProviders.hints.gcp',
  oracle: 'nodes:oracle.hint',
  custom: 'nodeProviders.hints.custom',
} as const

type KnownProvider = keyof typeof NAME_KEYS

function asKnown(name: string): KnownProvider | null {
  return name in NAME_KEYS ? (name as KnownProvider) : null
}

export function NodeProviderRow({
  provider,
}: {
  provider: NodeProviderResource
}) {
  const { t } = useTranslation(['settings', 'nodes'])
  const triggerRef = useRef<HTMLButtonElement>(null)
  const dialog = useUrlDialog(
    provider.provider,
    'connect',
    CONNECT_DIALOG_PARAMS,
  )
  const known = asKnown(provider.provider)
  const label = known ? t(NAME_KEYS[known]) : provider.provider
  const connected = provider.connected ?? provider.has_token
  const failed = provider.status === 'test_failed'
  const detail =
    provider.label && provider.provider === 'custom'
      ? [provider.label, provider.location].filter(Boolean).join(', ')
      : known
        ? t(HINT_KEYS[known])
        : ''

  return (
    <li className="relative flex flex-wrap items-center gap-x-3 gap-y-2 rounded-md py-3 hover:bg-muted/40">
      <BrandTile
        name={nodeProviderBrand(provider.provider)}
        label={label}
        labelled
      />
      <div className="min-w-0 flex-1 basis-56">
        <p className="text-sm font-medium">
          <Link
            to="/integrations/cloud/$provider"
            params={{ provider: provider.provider }}
            className="rounded-sm after:absolute after:inset-0 hover:underline focus-visible:outline-2 focus-visible:outline-offset-2"
          >
            {label}
          </Link>
        </p>
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
        {connected && provider.provider !== 'custom' ? (
          <ProviderRowStatus provider={provider.provider} />
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
        className="relative z-10"
        size="sm"
        aria-label={t('nodeProviders.configureFor', { provider: label })}
        onClick={() => {
          dialog.open()
        }}
      >
        <SettingsGlassIcon size="1.4em" aria-hidden="true" />
        {provider.has_token
          ? t('nodeProviders.update')
          : t('nodeProviders.connect')}
      </Button>
      <NodeProviderAccounts provider={provider} />
      <NodeProviderConnectDialog
        provider={provider}
        label={label}
        dialog={dialog}
        returnFocusRef={triggerRef}
      />
    </li>
  )
}
