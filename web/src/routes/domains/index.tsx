import { createFileRoute, Link, redirect } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { lazy, Suspense, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { GlobeGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { appListQueryOptions } from '../../queries/apps'
import { certificatesQueryOptions } from '../../queries/certificates'
import {
  domainsQueryOptions,
  ingressSettingsQueryOptions,
} from '../../queries/domains'
import { dashboardUrlQueryOptions } from '../../queries/dashboardUrl'
import { AddDomainsDialog } from '../../components/AddDomainsDialog'
import { ListKeyboardHint } from '../../components/ListKeyboardHint'
import { useListKeyboard } from '../../hooks/useListKeyboard'
import { DomainsTable } from '../../components/DomainsTable'
import { ParkedDomainsGroup } from '../../components/domains/ParkedDomainsGroup'
import { DomainsFirstRun } from '../../components/domains/DomainsFirstRun'
import { usePlatformStatus } from '../../components/platform/usePlatformStatus'
import { DomainsStatusBar } from '../../components/domains/DomainsStatusBar'
import { serverAddressQueryOptions } from '../../queries/serverAddress'
import { useProxyIntegration } from '../../queries/proxyIntegration'
import { proxyCertStatus } from '../../lib/certStatus'
import { DASHBOARD_URL_ANCHOR } from '../../components/DashboardUrlCard'
import { HelpLink } from '../../components/HelpLink'
import { PageBanner } from '../../components/PageBanner'
import { LazyDomainsDiagram } from '../../components/banners/LazyDiagrams'

const SavedViewsMenu = lazy(() => import('../../components/SavedViewsMenu'))
import { PageTemplate } from '../../components/shell/PageTemplate'

// The loader primes every query the page uses, so nothing fetches in a
// component body. Old /domains#dashboard-url links land on the new page.
export const Route = createFileRoute('/domains/')({
  validateSearch: (search: Record<string, unknown>): { q?: string } =>
    typeof search.q === 'string' && search.q !== '' ? { q: search.q } : {},
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
})

function DomainsPage() {
  const { t } = useTranslation('domains')
  const { data: domains } = useSuspenseQuery(domainsQueryOptions())
  const { data: certificates } = useSuspenseQuery(certificatesQueryOptions())
  const { data: apps } = useSuspenseQuery(appListQueryOptions())
  const [addOpen, setAddOpen] = useState(false)
  const {
    setContainer: setListContainer,
    hintOpen,
    closeHint,
  } = useListKeyboard<HTMLDivElement>()
  const query = Route.useSearch().q ?? ''
  const navigate = Route.useNavigate()
  const setQuery = (next: string) => {
    void navigate({ search: next ? { q: next } : {}, replace: true })
  }

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

  const { httpsOff } = usePlatformStatus()
  const noDomains = domains.length === 0
  const bannerSize = noDomains ? 'hero' : 'compact'
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
            <>
              {noDomains && apps.length === 0 ? (
                <Button
                  size="sm"
                  variant="outline"
                  render={<Link to="/apps/new" />}
                  nativeButton={false}
                >
                  {t('banner.noAppsAction')}
                </Button>
              ) : null}
              <HelpLink
                path="/domains-and-ingress"
                label={t('page.helpLabel')}
              />
            </>
          }
          illustration={<LazyDomainsDiagram size={bannerSize} />}
          size={bannerSize}
        />
      }
    >
      <DomainsStatusBar
        onAdd={() => {
          setAddOpen(true)
        }}
      />

      {noDomains ? (
        <DomainsFirstRun
          appCount={apps.length}
          domainCount={domains.length}
          httpsOff={httpsOff}
          onAdd={() => {
            setAddOpen(true)
          }}
        />
      ) : null}

      {noDomains ? null : (
        <>
          <ListKeyboardHint open={hintOpen} onClose={closeHint} />
          <div ref={setListContainer} className="contents">
            <DomainsTable
              domains={domains}
              certByDomain={certByDomain}
              appCount={apps.length}
              query={query}
              onQueryChange={setQuery}
              toolbar={
                <Suspense fallback={null}>
                  <SavedViewsMenu
                    list="domains"
                    current={{ q: query }}
                    onApply={(f) => {
                      setQuery(f.q ?? '')
                    }}
                  />
                </Suspense>
              }
              onAdd={() => {
                setAddOpen(true)
              }}
            />
          </div>
        </>
      )}

      <ParkedDomainsGroup />

      <AddDomainsDialog
        open={addOpen}
        onOpenChange={setAddOpen}
        apps={appChoices}
        claimedBy={claimedBy}
      />
    </PageTemplate>
  )
}
