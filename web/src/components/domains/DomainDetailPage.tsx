import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ChevronRightGlassIcon, RefreshCwGlassIcon } from '@thesvg/ui-react'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { VariantProps } from 'class-variance-authority'
import { certTruth } from '../../lib/certTruth'
import { certExpiryDate } from '../../lib/certStatus'
import { proxyCertTrusted } from '../../lib/dnsPill'
import { certificatesQueryOptions } from '../../queries/certificates'
import { DomainAuthGateControl } from '../DomainAuthGateControl'
import { DomainBasicAuthControl } from '../DomainBasicAuthControl'
import { DomainDnsRecordsControl } from '../DomainDnsRecordsControl'
import { DomainErrorPagesControl } from '../DomainErrorPagesControl'
import { DomainMaintenanceControl } from '../DomainMaintenanceControl'
import { DomainRedirectControl } from '../DomainRedirectControl'
import { DomainSearchVisibilityControl } from '../DomainSearchVisibilityControl'
import { DomainTLSCertControl } from '../DomainTLSCertControl'
import { DomainWafControl } from '../DomainWafControl'
import { DomainTrafficLimitsControl } from './DomainTrafficLimitsControl'
import { GetCertificateButton } from '../GetCertificateButton'
import { ProxyDomainStatus } from '../ProxyDomainStatus'
import {
  CertTimeline,
  DangerZone,
  DetailCard,
  DnsRecordCard,
  ResolverChips,
  TestItRow,
  type TimelineItem,
} from './DomainDetailParts'
import { pageState, type PageState } from './domainDetailModel'
import {
  allStepsDone,
  dnsPointsHere,
  readinessSteps,
  type ReadinessInput,
} from './domainReadiness'
import { DomainReadinessSteps } from './DomainReadinessSteps'
import { DomainRollbackStrip } from './DomainRollbackStrip'
import { useDomainCheckState } from './useDomainCheckState'
import { useRecheckCountdown } from './useRecheckCountdown'

const STATE_VARIANT: Record<
  PageState,
  VariantProps<typeof badgeVariants>['variant']
> = {
  live: 'success',
  waitingDns: 'warning',
  wrongDns: 'destructive',
  certFailed: 'destructive',
  issuing: 'info',
  starting: 'info',
}

// Settings for one domain as a full page: what to do on the left, options on the right.
export function DomainDetailPage({
  appName,
  domain,
}: {
  appName: string
  domain: string
}) {
  const { t } = useTranslation('domains')
  const { data, isFetching, onRefetch, proxyRow } = useDomainCheckState(
    appName,
    domain,
  )
  const { data: certs } = useQuery(certificatesQueryOptions())
  const cert = certs?.find((c) => c.domain === domain)
  const input: ReadinessInput | undefined = data && {
    status: data.status,
    verdict: data.verdict,
    tlsOwner: data.tls_owner,
    hasAcmeFailure: Boolean(data.acme_failure),
    certTrusted: proxyCertTrusted(proxyRow, data.proxy_certificate),
    proxyCertificate: data.proxy_certificate,
    proxyRow,
    route: data.route,
  }
  const state = input ? pageState(input) : undefined
  const live = state === 'live'
  const left = useRecheckCountdown({
    active: Boolean(input) && !live && !isFetching,
    resetKey: data?.checked_at,
    onFire: onRefetch,
  })
  const steps = input ? readinessSteps(input) : []
  const dnsOk = input ? dnsPointsHere(input) : false
  const certDone = steps[1]?.state === 'done'
  const expiresAt = cert?.not_after ?? data?.proxy_certificate?.not_after
  const timeline: TimelineItem[] = [
    { id: 'dns', done: dnsOk },
    { id: 'requested', done: certDone || steps[1]?.state === 'progress' },
    { id: 'issued', done: certDone },
    {
      id: 'expires',
      done: Boolean(expiresAt),
      detail: expiresAt ? certExpiryDate(expiresAt) : undefined,
    },
  ]
  const needsRealCert = certTruth(cert).wantsRealCert && !proxyRow

  return (
    <div className="space-y-4">
      <nav aria-label={t('detail.breadcrumb')}>
        <ol className="flex flex-wrap items-center gap-1 text-sm">
          <li>
            <Link
              to="/apps/$name/domains"
              params={{ name: appName }}
              className="text-muted-foreground hover:text-foreground"
            >
              {t('detail.back')}
            </Link>
          </li>
          <ChevronRightGlassIcon
            className="size-4.5 text-muted-foreground/50"
            aria-hidden="true"
          />
          <li aria-current="page" className="font-mono text-foreground">
            {domain}
          </li>
        </ol>
      </nav>

      <header className="sticky top-2 z-10 flex flex-wrap items-center justify-between gap-x-4 gap-y-2 rounded-lg border border-border bg-card px-4 py-3 shadow-sm">
        <div className="min-w-0">
          <h1 className="truncate font-mono text-base font-semibold">
            {domain}
          </h1>
          <p className="text-xs text-muted-foreground">
            {t('detail.description', { app: appName })}{' '}
            <Link
              to="/domains/$domain"
              params={{ domain }}
              search={{ app: appName }}
              className="underline"
            >
              {t('detail.allSettings')}
            </Link>
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-3" aria-live="polite">
          {state ? (
            <Badge variant={STATE_VARIANT[state]} className="text-sm">
              {t(`detail.state.${state}`)}
            </Badge>
          ) : (
            <Badge variant="muted">{t('page.dns.checking')}</Badge>
          )}
          {live || !input ? null : (
            <span className="text-xs text-muted-foreground">
              {isFetching
                ? t('dnsCheck.checking')
                : t('readiness.countdown', { seconds: left })}
            </span>
          )}
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={onRefetch}
            disabled={isFetching}
          >
            <RefreshCwGlassIcon
              className={isFetching ? 'animate-spin' : undefined}
              aria-hidden="true"
            />
            {t('readiness.checkNow')}
          </Button>
        </div>
      </header>

      <div className="grid gap-4 lg:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]">
        <div className="min-w-0 space-y-4">
          <DetailCard title={t('detail.routing')}>
            {data && input && !dnsOk ? (
              <DnsRecordCard domain={domain} data={data} />
            ) : null}
            {data ? <ResolverChips data={data} /> : null}
            <DomainRollbackStrip domain={domain} />
            <ProxyDomainStatus appName={appName} domain={domain} />
            <DomainDnsRecordsControl appName={appName} domain={domain} />
          </DetailCard>
          {data && input ? (
            <DetailCard title={t('readiness.title')}>
              <DomainReadinessSteps
                domain={domain}
                proxyRow={proxyRow}
                authoritative={data.authoritative}
                localStale={data.local_stale}
                checkedAt={data.checked_at}
                isFetching={isFetching}
                onRefetch={onRefetch}
                input={input}
                showControls={false}
              />
              {allStepsDone(steps) ? <TestItRow domain={domain} /> : null}
            </DetailCard>
          ) : null}
        </div>
        <div className="min-w-0 space-y-4">
          <DetailCard title={t('detail.tls')}>
            {dnsOk ? <CertTimeline items={timeline} /> : null}
            {needsRealCert ? <GetCertificateButton domain={domain} /> : null}
            <DomainTLSCertControl appName={appName} domain={domain} />
          </DetailCard>
          <DetailCard title={t('detail.redirect')}>
            <DomainRedirectControl appName={appName} domain={domain} />
          </DetailCard>
          <DetailCard title={t('moreOptions.title')}>
            <DomainBasicAuthControl appName={appName} domain={domain} />
            <DomainAuthGateControl appName={appName} domain={domain} />
            <DomainWafControl appName={appName} domain={domain} />
            <DomainMaintenanceControl appName={appName} domain={domain} />
            <DomainErrorPagesControl appName={appName} domain={domain} />
            <DomainSearchVisibilityControl appName={appName} domain={domain} />
            <DomainTrafficLimitsControl appName={appName} domain={domain} />
          </DetailCard>
          <DangerZone appName={appName} domain={domain} />
        </div>
      </div>
    </div>
  )
}
