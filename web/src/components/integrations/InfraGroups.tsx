import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  BoxesGlassIcon,
  BellGlassIcon,
  CloudGlassIcon,
  GitBranchGlassIcon,
  GlobeGlassIcon,
  ServerGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { StatusBadge } from '@/components/ui/status-badge'
import { isClustersOff, useClusters } from '../../queries/clusters'
import { useNodeProviders } from '../../queries/nodeProvision'
import type { NodeProviderStatus } from '../../types/nodeProvision'
import { CardGrid, ObjectCard } from '../cards/ObjectCard'
import { ConnectionGlyph } from './ConnectionGlyph'
import { providerStatus } from '../../lib/providerConnectionState'

const PROVIDER_TONE = {
  connected: 'success',
  not_connected: 'neutral',
  test_failed: 'danger',
} as const satisfies Record<
  NodeProviderStatus,
  'success' | 'neutral' | 'danger'
>

// Links to pages that already own each setup; the hub never forks their forms.
const LINK_GROUPS = [
  {
    id: 'git',
    to: '/settings/git-connections',
    icon: <GitBranchGlassIcon className="size-5" aria-hidden="true" />,
  },
  {
    id: 'dns',
    to: '/settings/platform/dns-providers',
    icon: <GlobeGlassIcon className="size-5" aria-hidden="true" />,
  },
  {
    id: 'tunnels',
    to: '/settings/cloudflare-tunnel',
    icon: <ServerGlassIcon className="size-5" aria-hidden="true" />,
  },
  {
    id: 'notifications',
    to: '/settings/notification-channels',
    icon: <BellGlassIcon className="size-5" aria-hidden="true" />,
  },
] as const

function ManageButton({
  to,
  params,
  label,
}: Readonly<{
  to:
    | '/settings/node-providers'
    | '/clusters'
    | (typeof LINK_GROUPS)[number]['to']
    | '/integrations/cloud/$provider'
  params?: { provider: string }
  label: string
}>) {
  const { t } = useTranslation('integrations')
  return (
    <Button
      variant="outline"
      size="sm"
      render={<Link to={to} params={params as never} />}
      aria-label={label}
    >
      {t('infra.manage')}
    </Button>
  )
}

export function InfraGroups() {
  const { t } = useTranslation('integrations')
  const providers = useNodeProviders()
  const clusters = useClusters()
  const clustersOn = !clusters.isError || !isClustersOff(clusters.error)
  const connectedClusters = clusters.data?.length

  return (
    <section aria-labelledby="integrations-infra" className="space-y-4">
      <h2
        id="integrations-infra"
        className="font-(family-name:--glinr-font-display) text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight) text-foreground"
      >
        {t('infra.title')}
      </h2>
      {providers.isPending ? null : (
        <div className="space-y-2">
          <h3 className="text-xs font-semibold text-muted-foreground">
            {t('infra.cloud')}
          </h3>
          <CardGrid min="md">
            {(providers.data ?? []).map((p) => {
              const status = providerStatus(p)
              const name = t(`infra.providers.${p.provider}`, {
                defaultValue: p.label ?? p.provider,
              })
              return (
                <li key={p.provider} className="flex min-w-0">
                  <ObjectCard
                    icon={
                      <ConnectionGlyph provider={p.provider} kind="cloud" />
                    }
                    title={name}
                    titleAs="h4"
                    status={
                      <StatusBadge
                        tone={PROVIDER_TONE[status]}
                        label={t(`infra.status.${status}`)}
                      />
                    }
                    inlineActions
                    actions={
                      <ManageButton
                        to="/integrations/cloud/$provider"
                        params={{ provider: p.provider }}
                        label={t('infra.manageNamed', { name })}
                      />
                    }
                  />
                </li>
              )
            })}
            {providers.data?.length ? null : (
              <li className="flex min-w-0">
                <ObjectCard
                  icon={
                    <CloudGlassIcon className="size-5" aria-hidden="true" />
                  }
                  title={t('infra.cloudAll')}
                  titleAs="h4"
                  inlineActions
                  actions={
                    <ManageButton
                      to="/settings/node-providers"
                      label={t('infra.manageNamed', {
                        name: t('infra.cloudAll'),
                      })}
                    />
                  }
                />
              </li>
            )}
          </CardGrid>
        </div>
      )}
      <CardGrid min="md">
        <li className="flex min-w-0">
          <ObjectCard
            icon={<BoxesGlassIcon className="size-5" aria-hidden="true" />}
            title={t('infra.clusters.title')}
            titleAs="h4"
            meta={
              clustersOn && connectedClusters !== undefined
                ? [t('infra.clusters.connected', { count: connectedClusters })]
                : [t('infra.clusters.off')]
            }
            inlineActions
            actions={
              <ManageButton
                to="/clusters"
                label={t('infra.manageNamed', {
                  name: t('infra.clusters.title'),
                })}
              />
            }
          />
        </li>
        {LINK_GROUPS.map((g) => (
          <li key={g.id} className="flex min-w-0">
            <ObjectCard
              icon={g.icon}
              title={t(`infra.links.${g.id}.title`)}
              titleAs="h4"
              meta={[t(`infra.links.${g.id}.hint`)]}
              inlineActions
              actions={
                <ManageButton
                  to={g.to}
                  label={t('infra.manageNamed', {
                    name: t(`infra.links.${g.id}.title`),
                  })}
                />
              }
            />
          </li>
        ))}
      </CardGrid>
    </section>
  )
}
