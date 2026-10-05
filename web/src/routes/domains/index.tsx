import { createFileRoute, Link } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useMemo, useRef } from 'react'
import { GlobeIcon } from '@phosphor-icons/react/dist/ssr'
import { appListQueryOptions } from '../../queries/apps'
import { certificatesQueryOptions } from '../../queries/certificates'
import { cloudflareDnsSettingsQueryOptions } from '../../queries/cloudflareDns'
import { route53DnsSettingsQueryOptions } from '../../queries/route53Dns'
import {
  domainsQueryOptions,
  ingressSettingsQueryOptions,
} from '../../queries/domains'
import {
  DOMAIN_LIST_GRID,
  DomainRow,
  RowSkeleton,
} from '../../components/DomainRow'
import { DomainAttentionStrip } from '../../components/DomainAttentionStrip'
import type { DomainAttentionEntry } from '../../components/DomainAttentionStrip'
import { certAttentionRank, sortByCertAttention } from '../../lib/certStatus'
import { CloudflareDnsCard } from '../../components/CloudflareDnsCard'
import { Route53DnsCard } from '../../components/Route53DnsCard'
import { IngressSettingsCard } from '../../components/IngressSettingsCard'
import { Button } from '../../components/ui/button'
import { DashboardUrlCard } from '../../components/DashboardUrlCard'
import { EnableHttpsCard } from '../../components/EnableHttpsCard'
import { FallbackDomainsCard } from '../../components/FallbackDomainsCard'
import { dashboardUrlQueryOptions } from '../../queries/dashboardUrl'
import { EmptyState } from '../../components/ui/empty-state'
import { PageHeader } from '../../components/shell/PageHeader'

// Centralized domains page: every domain currently claimed by an app
// (GET /api/v1/domains, service_domains) merged client-side with
// certificate status (GET /api/v1/certificates, already fetched for
// settings/general.tsx's own TLS card) by domain string, plus the
// platform-wide ingress settings (primary domain, ACME toggle) and the
// Cloudflare DNS-01 credential wildcard domains need on top of that.
// Typed loader primes all four queries before render, the same "no data
// fetching in the component body" rule every other route in this app
// follows.
export const Route = createFileRoute('/domains/')({
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.ensureQueryData(domainsQueryOptions()),
      queryClient.ensureQueryData(certificatesQueryOptions()),
      queryClient.ensureQueryData(ingressSettingsQueryOptions()),
      queryClient.ensureQueryData(cloudflareDnsSettingsQueryOptions()),
      queryClient.ensureQueryData(route53DnsSettingsQueryOptions()),
      queryClient.ensureQueryData(appListQueryOptions()),
      queryClient.ensureQueryData(dashboardUrlQueryOptions()),
    ]),
  component: DomainsPage,
  pendingComponent: DomainsPending,
})

function ListHeader() {
  return (
    <div
      className={`${DOMAIN_LIST_GRID} sticky top-0 z-10 border-b border-border bg-card px-4 py-2 text-xs font-medium tracking-wide text-muted-foreground uppercase`}
    >
      <span aria-hidden="true" />
      <span>Domain</span>
      <span>App</span>
      <span>Status</span>
      <span>Certificate</span>
      <span aria-hidden="true" />
    </div>
  )
}

function DomainsPage() {
  const { data: domains } = useSuspenseQuery(domainsQueryOptions())
  const { data: certificates } = useSuspenseQuery(certificatesQueryOptions())
  const { data: settings } = useSuspenseQuery(ingressSettingsQueryOptions())
  const { data: cloudflareDns } = useSuspenseQuery(
    cloudflareDnsSettingsQueryOptions(),
  )
  const { data: route53Dns } = useSuspenseQuery(
    route53DnsSettingsQueryOptions(),
  )
  const { data: apps } = useSuspenseQuery(appListQueryOptions())
  const parentRef = useRef<HTMLDivElement>(null)

  // Certificates are keyed by domain string (certificateStatus.domain,
  // internal/api/certificates.go), the same join key both this table
  // and IngressSettingsCard's own primary-domain lookup use. Built once
  // per render rather than a .find() per row, so this stays cheap at
  // whatever row count the virtualized list below is built to handle.
  const certByDomain = useMemo(() => {
    const m = new Map<string, (typeof certificates)[number]>()
    for (const cert of certificates) {
      m.set(cert.domain, cert)
    }
    return m
  }, [certificates])

  // Domains with a stalled renewal or non-healthy cert sort first (soonest
  // expiry first within that group), so they surface without scrolling on
  // a platform with many domains. Sorted before useVirtualizer sees it, so
  // virtualization measures the final row order.
  const sortedDomains = useMemo(
    () =>
      sortByCertAttention(domains, (domain) => certByDomain.get(domain.domain)),
    [domains, certByDomain],
  )

  const attentionEntries = useMemo(
    () =>
      sortedDomains.reduce<DomainAttentionEntry[]>((entries, domain) => {
        const cert = certByDomain.get(domain.domain)
        if (cert && certAttentionRank(cert) < 2) {
          entries.push({ domain, cert })
        }
        return entries
      }, []),
    [sortedDomains, certByDomain],
  )

  // Taller than the 60px other list pages use: this row's certificate
  // column can stack two badges (status + renewal-stalled), not just one.
  const virtualizer = useVirtualizer({
    count: sortedDomains.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 76,
    overscan: 8,
  })

  return (
    <div className="space-y-6">
      <PageHeader
        title="Domains"
        description="Every domain routed through this platform, and the ingress settings that decide how their certificates are issued."
        helpPath="/domains-and-ingress"
        helpLabel="Domains and ingress guide"
      />

      {!settings.primary_domain ||
      settings.primary_domain.endsWith('.sslip.io') ? (
        <EnableHttpsCard />
      ) : null}

      <IngressSettingsCard
        settings={settings}
        primaryCert={
          settings.primary_domain
            ? certByDomain.get(settings.primary_domain)
            : undefined
        }
      />

      <DashboardUrlCard />

      <FallbackDomainsCard />

      <CloudflareDnsCard settings={cloudflareDns} />

      <Route53DnsCard settings={route53Dns} />

      <DomainAttentionStrip entries={attentionEntries} />

      <div>
        <div className="mb-3 flex items-center gap-2">
          <GlobeIcon className="size-4 text-muted-foreground" />
          <h2 className="text-sm font-semibold text-foreground">App domains</h2>
          {domains.length > 0 ? (
            <span className="text-xs text-muted-foreground">
              {domains.length} {domains.length === 1 ? 'domain' : 'domains'}
            </span>
          ) : null}
        </div>

        {domains.length === 0 ? (
          apps.length === 0 ? (
            <EmptyState
              icon={<GlobeIcon className="size-5" />}
              title="No apps to route yet"
              description="A domain routes traffic to an app. Deploy an app first, then add a domain from its Domains tab."
              action={
                <Button
                  size="sm"
                  render={<Link to="/apps" />}
                  nativeButton={false}
                >
                  Deploy an app first
                </Button>
              }
            />
          ) : (
            <EmptyState
              icon={<GlobeIcon className="size-5" />}
              title="No app domains yet"
              description="Add a domain from an app's Domains tab to route traffic to it over HTTPS."
            />
          )
        ) : (
          <div
            ref={parentRef}
            className="h-[60vh] overflow-auto rounded-lg border border-border bg-card"
          >
            <ListHeader />
            <div
              style={{
                height: virtualizer.getTotalSize(),
                position: 'relative',
              }}
            >
              {virtualizer.getVirtualItems().map((row) => {
                const domain = sortedDomains[row.index]
                if (!domain) {
                  return null
                }
                return (
                  <div
                    key={domain.domain}
                    style={{
                      position: 'absolute',
                      top: 0,
                      left: 0,
                      width: '100%',
                      height: row.size,
                      transform: `translateY(${row.start}px)`,
                    }}
                  >
                    <DomainRow
                      domain={domain}
                      cert={certByDomain.get(domain.domain)}
                    />
                  </div>
                )
              })}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

// Route-level fallback for the loader's pending phase: the ingress and
// Cloudflare DNS cards below the header have no per-page skeleton of
// their own yet (both are single-form panels, not lists), so this shows
// generic card-shaped placeholders for them plus RowSkeleton rows for the
// app domains table, mirroring AppListPending's own reasoning.
function DomainsPending() {
  return (
    <div className="space-y-6">
      <PageHeader title="Domains" />
      <div className="h-32 animate-pulse rounded-lg border border-border bg-card" />
      <div className="h-24 animate-pulse rounded-lg border border-border bg-card" />
      <div>
        <div className="mb-3 flex items-center gap-2">
          <GlobeIcon className="size-4 text-muted-foreground" />
          <h2 className="text-sm font-semibold text-foreground">App domains</h2>
        </div>
        <div className="overflow-hidden rounded-lg border border-border bg-card">
          <ListHeader />
          {Array.from({ length: 6 }, (_, i) => (
            <RowSkeleton key={i} />
          ))}
        </div>
      </div>
    </div>
  )
}
