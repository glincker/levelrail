import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useMemo, useRef } from 'react'
import { GlobeIcon } from '@phosphor-icons/react/dist/ssr'
import { certificatesQueryOptions } from '../../queries/certificates'
import {
  domainsQueryOptions,
  ingressSettingsQueryOptions,
} from '../../queries/domains'
import { DOMAIN_LIST_GRID, DomainRow } from '../../components/DomainRow'
import { IngressSettingsCard } from '../../components/IngressSettingsCard'

// Centralized domains page: every domain currently claimed by an app
// (GET /api/v1/domains, service_domains) merged client-side with
// certificate status (GET /api/v1/certificates, already fetched for
// settings/general.tsx's own TLS card) by domain string, plus the
// platform-wide ingress settings (primary domain, ACME toggle) that used
// to have no UI at all. Typed loader primes all three queries before
// render, the same "no data fetching in the component body" rule every
// other route in this app follows.
export const Route = createFileRoute('/domains/')({
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.ensureQueryData(domainsQueryOptions()),
      queryClient.ensureQueryData(certificatesQueryOptions()),
      queryClient.ensureQueryData(ingressSettingsQueryOptions()),
    ]),
  component: DomainsPage,
})

function ListHeader() {
  return (
    <div
      className={`${DOMAIN_LIST_GRID} sticky top-0 z-10 border-b border-border bg-card px-4 py-2 text-xs font-medium tracking-wide text-muted-foreground uppercase`}
    >
      <span aria-hidden="true" />
      <span>Domain</span>
      <span>App</span>
      <span>Certificate</span>
      <span aria-hidden="true" />
    </div>
  )
}

function DomainsPage() {
  const { data: domains } = useSuspenseQuery(domainsQueryOptions())
  const { data: certificates } = useSuspenseQuery(certificatesQueryOptions())
  const { data: settings } = useSuspenseQuery(ingressSettingsQueryOptions())
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

  const virtualizer = useVirtualizer({
    count: domains.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 60,
    overscan: 8,
  })

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-semibold text-foreground">Domains</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Every domain routed through this platform, and the ingress
          settings that decide how their certificates are issued.
        </p>
      </div>

      <IngressSettingsCard
        settings={settings}
        primaryCert={
          settings.primary_domain
            ? certByDomain.get(settings.primary_domain)
            : undefined
        }
      />

      <div>
        <div className="mb-3 flex items-center gap-2">
          <GlobeIcon className="size-4 text-muted-foreground" />
          <h2 className="text-sm font-semibold text-foreground">
            App domains
          </h2>
          {domains.length > 0 ? (
            <span className="text-xs text-muted-foreground">
              {domains.length} {domains.length === 1 ? 'domain' : 'domains'}
            </span>
          ) : null}
        </div>

        {domains.length === 0 ? (
          <div className="rounded-lg border border-dashed border-border p-8 text-center text-sm text-muted-foreground">
            No app has a domain configured yet. Add one from an
            app&apos;s Domains tab.
          </div>
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
                const domain = domains[row.index]
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
