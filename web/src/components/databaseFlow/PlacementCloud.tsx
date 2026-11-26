import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  useNodeProviderRegions,
  useNodeProviders,
  useNodeProviderSizes,
} from '../../queries/nodeProvision'
import { BrandTile } from '../BrandLogoBadge'
import { nodeProviderBrand } from '../../lib/providerBrands'
import { cloudChoices, type CloudChoice } from './cloudChoices'
import { AddNodeWizard, type ProviderId } from '../AddNodeWizard'
import { PlusGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'

const WIZARD_IDS: readonly string[] = [
  'hetzner',
  'vultr',
  'digitalocean',
  'aws',
  'azure',
  'gcp',
  'oracle',
]

export function PlacementCloud() {
  const { t } = useTranslation('databases')
  const providers = useNodeProviders()
  const choices = cloudChoices(providers.data ?? [])
  const anyConnected = choices.some((c) => c.connected)
  const [picked, setPicked] = useState('')
  const provider = choices.find((c) => c.id === picked && c.connected)

  return (
    <div className="min-w-0 space-y-2 rounded-lg border border-dashed border-border p-2.5">
      {providers.isSuccess && !anyConnected ? (
        <div className="flex min-w-0 flex-wrap items-center justify-between gap-2">
          <p className="text-xs text-muted-foreground">
            {t('flow.placement.noCloud')}
          </p>
          <ConnectCloudLink />
        </div>
      ) : null}
      <div
        role="radiogroup"
        aria-label={t('flow.placement.kinds.cloud.title')}
        className="grid min-w-0 grid-cols-2 gap-2 min-[480px]:grid-cols-3"
      >
        {choices.map((c) => (
          <ProviderChip
            key={c.id}
            choice={c}
            selected={picked === c.id}
            onSelect={() => {
              setPicked(c.id)
            }}
          />
        ))}
      </div>
      {provider ? <CloudSizing provider={provider.id} /> : null}
      <p className="text-xs text-muted-foreground">
        {t('flow.placement.spendNote')}
      </p>
      <p className="text-xs text-muted-foreground">
        {t('flow.placement.combinedPending')}
      </p>
    </div>
  )
}

function ProviderChip({
  choice,
  selected,
  onSelect,
}: Readonly<{ choice: CloudChoice; selected: boolean; onSelect: () => void }>) {
  const { t } = useTranslation('databases')
  const label = t(`flow.placement.providers.${choice.id}`, {
    defaultValue: choice.id,
  })
  const status = !choice.wired
    ? t('flow.placement.comingSoon')
    : choice.connected
      ? t('flow.placement.connected')
      : t('flow.placement.notConnected')
  const body = (
    <>
      <BrandTile
        name={nodeProviderBrand(choice.id)}
        label={label}
        size="xs"
        muted={!choice.connected}
      />
      <span className="min-w-0">
        <span className="block truncate text-xs font-medium">{label}</span>
        <span className="block truncate text-xs text-muted-foreground">
          {status}
        </span>
      </span>
    </>
  )
  const base =
    'flex min-w-0 items-center gap-2 rounded-lg border border-input p-2 text-left focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none'
  if (choice.wired && !choice.connected) {
    return (
      <Link
        to="/integrations/cloud/$provider"
        params={{ provider: choice.id }}
        className={cn(base, 'hover:bg-muted')}
      >
        {body}
      </Link>
    )
  }
  return (
    <button
      type="button"
      role="radio"
      aria-checked={selected}
      disabled={!choice.wired}
      onClick={onSelect}
      className={cn(
        base,
        'hover:bg-muted disabled:cursor-not-allowed disabled:opacity-60',
        selected && 'border-primary bg-muted',
      )}
    >
      {body}
    </button>
  )
}

function CloudSizing({ provider }: Readonly<{ provider: string }>) {
  const { t } = useTranslation('databases')
  const [region, setRegion] = useState('')
  const [size, setSize] = useState('')
  const regions = useNodeProviderRegions(provider, true)
  const sizes = useNodeProviderSizes(provider, region, true)
  const chosen = sizes.data?.find((s) => s.id === size)
  const preselect: ProviderId | undefined = WIZARD_IDS.includes(provider)
    ? (provider as ProviderId)
    : undefined
  return (
    <div className="min-w-0 space-y-2">
      <div className="grid min-w-0 gap-2 sm:grid-cols-2">
        <div className="min-w-0 space-y-1">
          <Label htmlFor="placement-region">{t('flow.placement.region')}</Label>
          <Select
            value={region}
            onValueChange={(v) => {
              setRegion(v ?? '')
              setSize('')
            }}
          >
            <SelectTrigger id="placement-region" className="w-full">
              <SelectValue placeholder={t('flow.placement.pick')} />
            </SelectTrigger>
            <SelectContent>
              {(regions.data ?? []).map((r) => (
                <SelectItem key={r.id} value={r.id}>
                  {r.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="min-w-0 space-y-1">
          <Label htmlFor="placement-size">{t('flow.placement.size')}</Label>
          <Select
            value={size}
            disabled={region === ''}
            onValueChange={(v) => {
              setSize(v ?? '')
            }}
          >
            <SelectTrigger id="placement-size" className="w-full">
              <SelectValue placeholder={t('flow.placement.pick')} />
            </SelectTrigger>
            <SelectContent>
              {(sizes.data ?? []).map((s) => (
                <SelectItem key={s.id} value={s.id}>
                  {s.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>
      {chosen ? (
        <p className="text-xs text-muted-foreground">
          {chosen.price_monthly
            ? t('flow.placement.estimate', {
                price: chosen.price_monthly,
                currency: chosen.currency ?? '',
              })
            : t('flow.placement.noEstimate')}
        </p>
      ) : null}
      <AddNodeWizard
        preselect={preselect}
        triggerLabel={t('flow.placement.addCloudNode')}
      />
    </div>
  )
}

export function PlacementHost() {
  const { t } = useTranslation('databases')
  return (
    <div className="flex min-w-0 flex-wrap items-center justify-between gap-2 rounded-lg border border-dashed border-border p-2.5">
      <p className="min-w-0 flex-1 text-xs text-muted-foreground">
        {t('flow.placement.hostNote')}
      </p>
      <AddNodeWizard
        preselect="custom"
        triggerLabel={t('flow.placement.hostCommand')}
      />
    </div>
  )
}

export function ConnectCloudLink() {
  const { t } = useTranslation('databases')
  return (
    <Button
      variant="outline"
      size="sm"
      render={<Link to="/integrations" search={{ view: 'catalog' }} />}
    >
      <PlusGlassIcon size="1.4em" aria-hidden="true" />
      {t('flow.placement.connectCloud')}
    </Button>
  )
}
