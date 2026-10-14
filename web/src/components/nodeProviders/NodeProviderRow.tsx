import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { GearSixIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { BrandTile } from '../BrandLogoBadge'
import { StatusPill } from '../kit/StatusPill'
import { nodeProviderBrand } from '../../lib/providerBrands'
import type { NodeProviderResource } from '../../types/nodeProvision'
import { NodeProviderForm } from './NodeProviderForm'

const KNOWN_PROVIDERS = [
  'hetzner',
  'digitalocean',
  'aws',
  'azure',
  'gcp',
] as const
type KnownProvider = (typeof KNOWN_PROVIDERS)[number]

function asKnown(name: string): KnownProvider | null {
  return KNOWN_PROVIDERS.find((k) => k === name) ?? null
}

export function NodeProviderRow({
  provider,
}: {
  provider: NodeProviderResource
}) {
  const { t } = useTranslation('settings')
  const [open, setOpen] = useState(false)
  const known = asKnown(provider.provider)
  const label = known ? t(`nodeProviders.names.${known}`) : provider.provider
  const connected = provider.has_token

  return (
    <li className="flex flex-wrap items-center gap-x-3 gap-y-2 py-3">
      <BrandTile
        name={nodeProviderBrand(provider.provider)}
        label={label}
        labelled
      />
      <div className="min-w-0 flex-1 basis-56">
        <p className="text-sm font-medium">{label}</p>
        {known ? (
          <p className="truncate text-xs text-muted-foreground">
            {t(`nodeProviders.hints.${known}`)}
          </p>
        ) : null}
      </div>
      <StatusPill
        tone={connected ? 'success' : 'neutral'}
        label={
          connected
            ? t('nodeProviders.connected')
            : t('nodeProviders.notConnected')
        }
        size="sm"
      />
      <Button
        type="button"
        variant="outline"
        size="sm"
        aria-label={t('nodeProviders.configureFor', { provider: label })}
        onClick={() => setOpen(true)}
      >
        <GearSixIcon aria-hidden="true" />
        {connected ? t('nodeProviders.update') : t('nodeProviders.connect')}
      </Button>
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent className="data-[side=right]:sm:max-w-xl">
          <SheetHeader className="border-b border-border pr-12">
            <SheetTitle>
              {t('nodeProviders.sheetTitle', { provider: label })}
            </SheetTitle>
            <SheetDescription>
              {t('nodeProviders.sheetDescription')}
            </SheetDescription>
          </SheetHeader>
          <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-4">
            <NodeProviderForm
              provider={provider}
              label={label}
              onSaved={() => setOpen(false)}
            />
          </div>
        </SheetContent>
      </Sheet>
    </li>
  )
}
