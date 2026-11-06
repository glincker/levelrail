import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, CopyGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import { proxyCertTrusted } from '../../lib/dnsPill'
import { findProxyDomain } from '../../lib/proxyIntegration'
import { wizardRecords, type DomainKind } from '../../lib/domainWizard'
import { useDomainCheck } from '../../queries/domainCheck'
import { useRunGoLive } from '../../queries/goLive'
import { proxyIntegrationQueryOptions } from '../../queries/proxyIntegration'
import { DomainReadinessSteps } from './DomainReadinessSteps'
import { DomainWwwCard } from './DomainWwwCard'
import { allStepsDone, readinessSteps } from './domainReadiness'

function CopyRow({ label, value }: { label: string; value: string }) {
  const { t } = useTranslation('domains')
  const { copied, copy } = useCopyToClipboard()
  return (
    <div className="flex items-center gap-2 rounded-md border border-input bg-background p-1.5">
      <span className="w-12 shrink-0 text-xs font-medium text-muted-foreground">
        {label}
      </span>
      <code className="min-w-0 flex-1 text-xs break-all">{value}</code>
      <Button
        type="button"
        size="icon-sm"
        variant="ghost"
        aria-label={
          copied
            ? t('addWizard.step2.copied', { label })
            : t('addWizard.step2.copy', { label })
        }
        onClick={() => {
          copy(value)
        }}
      >
        {copied ? (
          <CheckGlassIcon className="size-5" aria-hidden="true" />
        ) : (
          <CopyGlassIcon className="size-5" aria-hidden="true" />
        )}
      </Button>
    </div>
  )
}

// Step 2: the exact records, plus a one click apply only when a provider is connected.
export function PointStep({
  app,
  domain,
  kind,
  onContinue,
  onBack,
}: {
  app: string
  domain: string
  kind: DomainKind
  onContinue: () => void
  onBack: () => void
}) {
  const { t } = useTranslation('domains')
  const check = useDomainCheck(app, domain, { poll: false })
  const goLive = useRunGoLive(app, domain)
  const data = check.data
  const rows = wizardRecords(
    domain,
    kind,
    data?.expected_host,
    data?.expected_ipv4?.[0],
    data?.expected_ipv6?.[0],
  )
  const hasProvider = Boolean(
    data?.dns_provider && data.dns_provider !== 'none',
  )
  const applied = goLive.isSuccess
  const applyFirst = hasProvider && !applied

  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">
        {t('addWizard.step2.intro', { domain })}
      </p>
      {check.isPending ? (
        <p className="text-sm text-muted-foreground" aria-live="polite">
          {t('addWizard.step2.loading')}
        </p>
      ) : rows.length === 0 ? (
        <p className="text-sm text-tone-danger" role="alert">
          {t('addWizard.step2.noRecords')}
        </p>
      ) : (
        <div className="space-y-2">
          {rows.map((r) => (
            <div key={r.type} className="space-y-1">
              <CopyRow label={t('addWizard.step2.type')} value={r.type} />
              <CopyRow label={t('addWizard.step2.name')} value={r.name} />
              <CopyRow label={t('addWizard.step2.value')} value={r.value} />
            </div>
          ))}
        </div>
      )}
      <p className="text-sm" aria-live="polite">
        {goLive.isSuccess
          ? t('addWizard.step2.applied')
          : hasProvider
            ? t('addWizard.step2.providerOn')
            : t('addWizard.step2.providerOff')}
      </p>
      {goLive.isError ? (
        <p className="text-sm text-tone-danger" role="alert">
          {t('addWizard.step2.applyFailed', { message: goLive.error.message })}
        </p>
      ) : null}
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Button type="button" variant="ghost" onClick={onBack}>
          {t('addWizard.back')}
        </Button>
        {applyFirst ? (
          <Button
            type="button"
            disabled={goLive.isPending}
            onClick={() => {
              goLive.mutate({})
            }}
          >
            {goLive.isPending
              ? t('addWizard.step2.applying')
              : t('addWizard.step2.apply')}
          </Button>
        ) : (
          <Button type="button" onClick={onContinue}>
            {t('addWizard.step2.continue')}
          </Button>
        )}
      </div>
      {applyFirst ? (
        <Button type="button" variant="link" size="sm" onClick={onContinue}>
          {t('addWizard.step2.continue')}
        </Button>
      ) : null}
    </div>
  )
}

// Step 3: live readiness with its countdown; success only once every step is done.
export function LiveStep({
  app,
  domain,
  kind,
  onBack,
  onAddAnother,
  onFinishLater,
}: {
  app: string
  domain: string
  kind: DomainKind
  onBack: () => void
  onAddAnother: () => void
  onFinishLater: () => void
}) {
  const { t } = useTranslation('domains')
  const check = useDomainCheck(app, domain, { poll: false })
  const proxy = useQuery(proxyIntegrationQueryOptions())
  const data = check.data
  const proxyRow = findProxyDomain(proxy.data, domain)
  const input = data
    ? {
        status: data.status,
        verdict: data.verdict,
        tlsOwner: data.tls_owner,
        hasAcmeFailure: Boolean(data.acme_failure),
        certTrusted: proxyCertTrusted(proxyRow, data.proxy_certificate),
        proxyCertificate: data.proxy_certificate,
        proxyRow,
        route: data.route,
      }
    : null
  const done = input ? allStepsDone(readinessSteps(input)) : false

  if (done) {
    return (
      <div className="space-y-3">
        <div aria-live="polite">
          <p className="font-medium text-tone-success">
            {t('addWizard.step3.doneTitle', { domain })}
          </p>
          <p className="text-sm text-muted-foreground">
            {t('addWizard.step3.doneBody')}
          </p>
        </div>
        {kind === 'apex' ? (
          <DomainWwwCard appName={app} domain={domain} />
        ) : null}
        <div className="flex flex-wrap items-center justify-between gap-2">
          <Button type="button" variant="outline" onClick={onAddAnother}>
            {t('addWizard.step3.addAnother')}
          </Button>
          <Button
            render={
              <a href={`https://${domain}`} target="_blank" rel="noreferrer" />
            }
            nativeButton={false}
          >
            {t('addWizard.step3.open', { domain })}
          </Button>
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">
        {t('addWizard.step3.waiting')}
      </p>
      {input ? (
        <DomainReadinessSteps
          domain={domain}
          proxyRow={proxyRow}
          authoritative={data?.authoritative}
          localStale={data?.local_stale}
          checkedAt={data?.checked_at}
          isFetching={check.isFetching}
          onRefetch={() => {
            void check.refetch()
          }}
          input={input}
        />
      ) : null}
      {kind === 'apex' ? (
        <div className="space-y-1">
          <p className="text-sm font-medium">{t('addWizard.step3.wwwTitle')}</p>
          <p className="text-xs text-muted-foreground">
            {t('addWizard.step3.wwwBody', { domain })}
          </p>
          <DomainWwwCard appName={app} domain={domain} />
        </div>
      ) : null}
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Button type="button" variant="ghost" onClick={onBack}>
          {t('addWizard.back')}
        </Button>
        <Button type="button" variant="outline" onClick={onFinishLater}>
          {t('addWizard.step3.finishLater')}
        </Button>
      </div>
    </div>
  )
}
