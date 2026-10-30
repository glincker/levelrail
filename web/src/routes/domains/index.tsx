import { createFileRoute, Link, redirect } from '@tanstack/react-router'
import type { ErrorComponentProps } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CircleAlertGlassIcon,
  GlobeGlassIcon,
  PlusGlassIcon,
} from '@thesvg/ui-react'
import { routeErrorMessage } from '@/lib/apiError'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { appListQueryOptions } from '../../queries/apps'
import { certificatesQueryOptions } from '../../queries/certificates'
import {
  domainsQueryOptions,
  ingressSettingsQueryOptions,
} from '../../queries/domains'
import { dashboardUrlQueryOptions } from '../../queries/dashboardUrl'
import { AddDomainsDialog } from '../../components/AddDomainsDialog'
import {
  DomainsTable,
  DomainsTableSkeleton,
} from '../../components/DomainsTable'
import { ProxySetupCard } from '../../components/ProxySetupCard'
import { ServerAddressCard } from '../../components/domains/ServerAddressCard'
import { serverAddressQueryOptions } from '../../queries/serverAddress'
import { useProxyIntegration } from '../../queries/proxyIntegration'
import { proxyCertStatus } from '../../lib/certStatus'
import { PlatformStatusStrip } from '../../components/platform/PlatformStatusStrip'
import { DASHBOARD_URL_ANCHOR } from '../../components/DashboardUrlCard'
import { PageBanner } from '../../components/PageBanner'
import { LazyDomainsDiagram } from '../../components/banners/LazyDiagrams'
import { PageTemplate } from '../../components/shell/PageTemplate'
import { SetupStepHint } from '../../components/getStarted/SetupStepHint'

// The loader primes every query the page uses, so nothing fetches in a
// component body. Old /domains#dashboard-url links land on the new page.
export const Route = createFileRoute('/domains/')({
  beforeLoad: ({ location }) => {
    if (location.hash === DASHBOARD_URL_ANCHOR) {
      redirect({
        to: '/settings/platform/addresses',
        hash: DASHBOARD_URL_ANCHOR,
        replace: true,
        throw: true,
      })
    }
  },
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.ensureQueryData(domainsQueryOptions()),
      queryClient.ensureQueryData(certificatesQueryOptions()),
      queryClient.ensureQueryData(ingressSettingsQueryOptions()),
      queryClient.ensureQueryData(appListQueryOptions()),
      queryClient.ensureQueryData(dashboardUrlQueryOptions()),
      queryClient.prefetchQuery(serverAddressQueryOptions()),
    ]),
  component: DomainsPage,
  pendingComponent: DomainsPending,
  errorComponent: DomainsError,
})

function DomainsPage() {
  const { t } = useTranslation('domains')
  const { data: domains } = useSuspenseQuery(domainsQueryOptions())
  const { data: certificates } = useSuspenseQuery(certificatesQueryOptions())
  const { data: apps } = useSuspenseQuery(appListQueryOptions())
  const [addOpen, setAddOpen] = useState(false)

  const { data: proxy } = useProxyIntegration()
  const certByDomain = useMemo(() => {
    const byDomain = new Map(certificates.map((cert) => [cert.domain, cert]))
    for (const d of proxy?.domains ?? []) {
      const seen = d.certificate
        ? proxyCertStatus(d.domain, d.certificate)
        : undefined
      if (seen) byDomain.set(d.domain, seen)
    }
    return byDomain
  }, [certificates, proxy])
  const claimedBy = useMemo(
    () =>
      new Map(
        domains.map((d) => [
          d.domain,
          d.kind === 'standalone' ? (d.target ?? d.domain) : d.service_name,
        ]),
      ),
    [domains],
  )
  const appChoices = useMemo(
    () => apps.map((app) => ({ name: app.name })),
    [apps],
  )

  const noDomains = domains.length === 0
  const bannerSize = noDomains ? 'hero' : 'compact'
  const addButton = (
    <Button
      size="sm"
      onClick={() => {
        setAddOpen(true)
      }}
    >
      <PlusGlassIcon size="1.4em" />
      {t('page.add')}
    </Button>
  )

  return (
    <PageTemplate
      title={t('page.title')}
      description={t('page.description')}
      banner={
        <PageBanner
          icon={<GlobeGlassIcon size="1.4em" />}
          titleKey="domains:page.title"
          descriptionKey={
            noDomains
              ? 'domains:banner.description'
              : 'domains:page.description'
          }
          action={
            noDomains && apps.length === 0 ? (
              <>
                <p className="w-full text-sm text-foreground">
                  {t('banner.noApps')}
                </p>
                <Button
                  size="sm"
                  variant="outline"
                  render={<Link to="/apps/new" />}
                  nativeButton={false}
                >
                  {t('banner.noAppsAction')}
                </Button>
                {addButton}
              </>
            ) : (
              addButton
            )
          }
          illustration={<LazyDomainsDiagram size={bannerSize} />}
          size={bannerSize}
        />
      }
      helpPath="/domains-and-ingress"
      helpLabel="Domains and ingress guide"
      banners={<SetupStepHint step="app_domain" />}
      actions={addButton}
    >
      <PlatformStatusStrip />

      <ProxySetupCard />

      <ServerAddressCard />

      {noDomains ? null : (
        <DomainsTable
          domains={domains}
          certByDomain={certByDomain}
          appCount={apps.length}
          onAdd={() => {
            setAddOpen(true)
          }}
        />
      )}

      <AddDomainsDialog
        open={addOpen}
        onOpenChange={setAddOpen}
        apps={appChoices}
        claimedBy={claimedBy}
      />
    </PageTemplate>
  )
}

function DomainsPending() {
  const { t } = useTranslation('domains')
  return (
    <PageTemplate title={t('page.title')}>
      <DomainsTableSkeleton />
    </PageTemplate>
  )
}

function DomainsError({ error, reset }: ErrorComponentProps) {
  const { t } = useTranslation('domains')
  return (
    <PageTemplate title={t('page.title')}>
      <EmptyState
        icon={<CircleAlertGlassIcon className="size-6 text-tone-danger" />}
        title={t('page.error.title')}
        description={routeErrorMessage(error)}
        action={
          <Button size="sm" onClick={reset}>
            {t('page.error.retry')}
          </Button>
        }
      />
    </PageTemplate>
  )
}
