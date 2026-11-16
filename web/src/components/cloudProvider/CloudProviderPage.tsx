import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { PlusGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { AddNodeWizard } from '../AddNodeWizard'
import { BrandTile } from '../BrandLogoBadge'
import {
  cloudProviderFor,
  type CloudProviderId,
  type CloudProviderPresentation,
} from '../../lib/cloudProviderBrand'
import {
  useNodeProviderRegions,
  useNodeProviders,
} from '../../queries/nodeProvision'
import type { NodeProviderResource } from '../../types/nodeProvision'
import { ProviderLinks } from './ProviderLinks'
import { SECTIONS, SECTION_ORDER } from './sections'
import { useProviderLabels } from './useProviderLabels'

function UnknownProvider() {
  const { t } = useTranslation('cloudProvider')
  return (
    <PageTemplate title={t('page.notFound.title')}>
      <p className="text-sm text-muted-foreground">{t('page.notFound.body')}</p>
      <Button variant="outline" size="sm" render={<Link to="/integrations" />}>
        {t('links.manage')}
      </Button>
    </PageTemplate>
  )
}

export function CloudProviderPage({
  providerId,
  builds = false,
}: Readonly<{ providerId: string; builds?: boolean }>) {
  const provider = cloudProviderFor(providerId)
  if (!provider) return <UnknownProvider />
  return <ProviderLanding provider={provider} builds={builds} />
}

function ProviderLanding({
  provider,
  builds,
}: Readonly<{ provider: CloudProviderPresentation; builds: boolean }>) {
  const { t } = useTranslation('cloudProvider')
  const labels = useProviderLabels(provider.id)
  const providers = useNodeProviders()
  const resource: NodeProviderResource = providers.data?.find(
    (p) => p.provider === provider.id,
  ) ?? { provider: provider.id, has_token: false }
  const manual = provider.id === 'custom'
  const connected = resource.has_token
  const regions = useNodeProviderRegions(
    provider.id,
    connected && provider.quickFactRegions,
  )
  const canCreate =
    provider.features.includes('create') && (connected || manual)

  return (
    <PageTemplate
      title={labels.name}
      description={labels.tagline}
      logo={
        <BrandTile
          name={provider.brand}
          label={labels.name}
          size="xl"
          labelled
        />
      }
      breadcrumb={
        <Link
          to="/integrations"
          className="text-xs text-muted-foreground hover:text-foreground hover:underline"
        >
          {t('page.back')}
        </Link>
      }
      actions={
        provider.features.includes('create') ? (
          canCreate ? (
            <AddNodeWizard
              preselect={provider.id}
              builds={builds}
              triggerLabel={labels.createLabel}
            />
          ) : (
            <Button type="button" size="sm" disabled>
              <PlusGlassIcon size="1.4em" aria-hidden="true" />
              {labels.createLabel}
            </Button>
          )
        ) : undefined
      }
    >
      <div className="relative space-y-4">
        {provider.glowClass ? (
          <div
            aria-hidden="true"
            className={cn(
              'pointer-events-none absolute inset-x-0 -top-24 -z-10 h-40 bg-linear-to-b to-transparent',
              provider.glowClass,
            )}
          />
        ) : null}
        <ul
          className="flex flex-wrap items-center gap-1.5"
          aria-label={labels.name}
        >
          {labels.vocab.split(', ').map((term) => (
            <li
              key={term}
              className={cn(
                'rounded-full border px-2 py-0.5 text-xs',
                provider.chipClass,
              )}
            >
              {term}
            </li>
          ))}
          {regions.data?.length ? (
            <li className="text-xs text-muted-foreground">
              {t('page.quickFacts.regions', {
                count: regions.data.length,
                label: labels.locations.toLowerCase(),
              })}
            </li>
          ) : null}
        </ul>
        {SECTION_ORDER.filter((f) => provider.features.includes(f)).map((f) => {
          const Section = SECTIONS[f]
          return Section ? (
            <Section
              key={f}
              provider={provider}
              resource={resource}
              labels={labels}
            />
          ) : null
        })}
        <ProviderLinks provider={provider} labels={labels} />
      </div>
    </PageTemplate>
  )
}

export type { CloudProviderId }
