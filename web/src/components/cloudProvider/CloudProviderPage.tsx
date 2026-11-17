import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { PlugGlassIcon, PlusGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { StatusPill } from '../kit/StatusPill'
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
import { CloudProvidersBreadcrumb } from './CloudProvidersBreadcrumb'
import { ProviderLinks } from './ProviderLinks'
import { ProviderOnboarding } from './ProviderOnboarding'
import { RAIL_SECTIONS, SECTIONS, SECTION_ORDER } from './sections'
import { useProviderConnection } from './useProviderConnection'
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
  const connection = useProviderConnection(provider, resource, labels.name)
  const failed = resource.status === 'test_failed'
  const sectionProps = { provider, resource, labels, connection }
  const render = (zone: 'rail' | 'main') =>
    SECTION_ORDER.filter(
      (f) =>
        provider.features.includes(f) &&
        RAIL_SECTIONS.includes(f) === (zone === 'rail'),
    ).map((f) => {
      const Section = SECTIONS[f]
      return Section ? <Section key={f} {...sectionProps} /> : null
    })
  // Without a credential the other sections would all be empty, so one card replaces them.
  const onboarding = !connected && !manual
  const rail = onboarding ? null : (
    <>
      {render('rail')}
      <ProviderLinks provider={provider} labels={labels} />
    </>
  )

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
      status={
        manual ? undefined : (
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
        )
      }
      breadcrumb={<CloudProvidersBreadcrumb current={labels.name} />}
      banners={
        <p className="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-muted-foreground">
          <span
            aria-hidden="true"
            className={cn('size-1.5 rounded-full', provider.dotClass)}
          />
          {labels.vocab !== labels.servers ? (
            <span>{labels.vocab.split(', ').join(' · ')}</span>
          ) : null}
          {regions.data?.length ? (
            <span>
              {t('page.quickFacts.regions', {
                count: regions.data.length,
                label: labels.locations.toLowerCase(),
              })}
            </span>
          ) : null}
        </p>
      }
      actions={
        <>
          {onboarding ? (
            <Button type="button" size="sm" onClick={connection.openConnect}>
              <PlugGlassIcon size="1.4em" aria-hidden="true" />
              {t('connection.connect')}
            </Button>
          ) : null}
          {provider.features.includes('create') && !onboarding ? (
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
          ) : null}
          {connection.dialogs}
        </>
      }
      aside={rail}
      asideLabel={t('page.railLabel', { name: labels.name })}
    >
      {onboarding ? <ProviderOnboarding {...sectionProps} /> : render('main')}
    </PageTemplate>
  )
}

export type { CloudProviderId }
