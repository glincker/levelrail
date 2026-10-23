import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowSquareOutIcon,
  CheckIcon,
  CopyIcon,
  DotsThreeIcon,
  GearSixIcon,
  ListMagnifyingGlassIcon,
  LockKeyIcon,
  ShieldCheckIcon,
  SignpostIcon,
  StethoscopeIcon,
  WarningCircleIcon,
  WrenchIcon,
  ArrowsClockwiseIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import type { TFunction } from 'i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import {
  certExpiryDate,
  certExpiryLabel,
  certRenewalBadge,
  effectiveDomainCert,
  isInternalCert,
  isProxyManagedCert,
} from '../lib/certStatus'
import { useCopyToClipboard } from '../hooks/useCopyToClipboard'
import { certTruth } from '../lib/certTruth'
import { CertTruthBadge } from './CertTruthBadge'
import { GoLiveBadge } from './golive/GoLiveBadge'
import { GetCertificateButton } from './GetCertificateButton'
import type { CertificateStatus } from '../queries/certificates'
import { domainCheckQueryOptions } from '../queries/domainCheck'
import type { DomainCheckResult } from '../queries/domainCheck'
import {
  useDomainSearchVisibility,
  useSetDomainSearchVisibility,
} from '../queries/domainSearchVisibility'
import type { Domain } from '../queries/domains'
import { domainScope, isStandaloneDomain } from '../lib/standaloneDomain'
import { isEnforced } from '../lib/domainEnforcement'
import type { TrafficControl } from '../lib/domainEnforcement'
import { DomainDnsRecordDialog } from './DomainDnsRecordDialog'
import { DomainIssueFix } from './DomainIssueFix'
import { DomainDnsStatusBadge } from './DomainDnsStatusBadge'
import { DomainDiagnoseDialog } from './domains/DomainDiagnoseDialog'
import {
  StandaloneRowActions,
  StandaloneTargetCell,
} from './domains/StandaloneRowParts'

const FLAGS: {
  key: keyof Pick<
    Domain,
    'waf_enabled' | 'has_redirect' | 'maintenance_enabled' | 'has_basic_auth'
  >
  control: TrafficControl
  icon: Icon
  label:
    | 'page.flag.waf'
    | 'page.flag.redirect'
    | 'page.flag.maintenance'
    | 'page.flag.basicAuth'
}[] = [
  {
    key: 'waf_enabled',
    control: 'waf',
    icon: ShieldCheckIcon,
    label: 'page.flag.waf',
  },
  {
    key: 'has_redirect',
    control: 'redirect',
    icon: SignpostIcon,
    label: 'page.flag.redirect',
  },
  {
    key: 'maintenance_enabled',
    control: 'maintenance',
    icon: WrenchIcon,
    label: 'page.flag.maintenance',
  },
  {
    key: 'has_basic_auth',
    control: 'basic_auth',
    icon: LockKeyIcon,
    label: 'page.flag.basicAuth',
  },
]

// First problem wins, so a row carries at most one inline warning.
function inlineWarning(
  domain: Domain,
  cert: CertificateStatus | undefined,
  dns: DomainCheckResult | undefined,
  t: TFunction<'domains'>,
): { text: string; title: string } | null {
  const issue = domain.issues?.[0]
  if (issue) {
    const text = t(`page.issue.${issue.code}`, { defaultValue: issue.message })
    return { text, title: issue.message }
  }
  const proxyOwned = domain.tls_owner === 'proxy'
  const failure = proxyOwned
    ? undefined
    : (cert?.acme_failure ?? domain.acme_failure)
  if (failure) {
    const text = t(`acme.action.${failure.action}`)
    return { text, title: `${failure.error}\n${text}` }
  }
  if (!dns) return null
  if (dns.status === 'not_resolving') {
    const text = t('page.dns.notResolvingHint')
    return { text, title: text }
  }
  if (dns.status === 'resolves_elsewhere') {
    const hosts = dns.resolved_hosts?.join(', ')
    const text = hosts
      ? t('page.dns.elsewhereHint', { hosts })
      : t('page.dns.elsewhereHintNoHosts')
    return { text, title: text }
  }
  if (dns.expected_private && !proxyOwned) {
    const text = t('page.dns.privateHint')
    return { text, title: text }
  }
  return null
}

function SearchVisibilityCell({ domain }: { domain: Domain }) {
  const { t } = useTranslation('domains')
  const { data, isLoading } = useDomainSearchVisibility(
    domainScope(domain),
    domain.domain,
  )
  const set = useSetDomainSearchVisibility(domainScope(domain), domain.domain)
  if (isLoading) return <Skeleton className="h-5 w-9" />
  const visible = !(data?.hidden ?? false)
  return (
    <Switch
      size="sm"
      checked={visible}
      disabled={set.isPending || !data}
      aria-label={t('page.visibility.toggle', { domain: domain.domain })}
      title={
        visible ? t('page.visibility.visible') : t('page.visibility.hidden')
      }
      onCheckedChange={(next) => {
        set.mutate(!next, {
          onError: () => {
            toast.add({
              title: t('page.visibility.failed', { domain: domain.domain }),
              type: 'error',
            })
          },
        })
      }}
    />
  )
}

function CertCell({
  domain,
  cert,
}: {
  domain: Domain
  cert?: CertificateStatus
}) {
  const { t } = useTranslation('domains')
  if (!cert && domain.tls_owner === 'proxy') {
    return <Badge variant="info">{t('proxyManaged.badge')}</Badge>
  }
  const truth = certTruth(cert)
  const renewal = cert ? certRenewalBadge(cert) : null
  return (
    <span className="flex min-w-0 flex-col items-start gap-1">
      {!cert ? (
        domain.acme_failure ? (
          <Badge variant="destructive">{t('page.cert.failed')}</Badge>
        ) : (
          <span className="text-xs text-muted-foreground/70">
            {t('page.cert.none')}
          </span>
        )
      ) : (
        <CertTruthBadge cert={cert} />
      )}
      {renewal ? (
        <Badge variant="destructive" title={renewal.hint}>
          {renewal.label}
        </Badge>
      ) : null}
      {truth.wantsRealCert && domain.tls_owner !== 'proxy' ? (
        <GetCertificateButton domain={domain.domain} />
      ) : null}
      {isStandaloneDomain(domain) ? null : (
        <GoLiveBadge app={domain.service_name} domain={domain.domain} />
      )}
    </span>
  )
}

function ExpiryCell({ cert }: { cert?: CertificateStatus }) {
  const { t } = useTranslation('domains')
  if (!cert) return <span className="text-xs text-muted-foreground/60">-</span>
  if (isInternalCert(cert)) {
    return (
      <span className="text-xs text-muted-foreground">
        {t('page.cert.internal')}
      </span>
    )
  }
  const renews =
    cert.source === 'custom'
      ? t('page.cert.renewsManual')
      : isProxyManagedCert(cert)
        ? t('page.cert.renewsProxy')
        : t('page.cert.renewsAuto')
  return (
    <span
      className="flex min-w-0 flex-col text-xs text-muted-foreground"
      title={cert.not_after}
    >
      <span className="truncate text-foreground">
        {certExpiryDate(cert.not_after)}
      </span>
      <span className="truncate">{certExpiryLabel(cert.not_after)}</span>
      <span className="truncate text-muted-foreground/70">{renews}</span>
    </span>
  )
}

function RowActions({
  domain,
  onShowRecord,
  onRecheck,
  onDiagnose,
}: {
  domain: Domain
  onShowRecord: () => void
  onRecheck: () => void
  onDiagnose: () => void
}) {
  const { t } = useTranslation('domains')
  const { copied, copy } = useCopyToClipboard()
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            className="justify-self-end"
            aria-label={t('page.row.actions', { domain: domain.domain })}
          />
        }
      >
        <DotsThreeIcon weight="bold" />
      </DropdownMenuTrigger>
      <DropdownMenuContent>
        <DropdownMenuItem onClick={onShowRecord}>
          <ListMagnifyingGlassIcon />
          {t('page.row.dnsRecord')}
        </DropdownMenuItem>
        <DropdownMenuItem onClick={onDiagnose}>
          <StethoscopeIcon />
          {t('page.row.diagnose')}
        </DropdownMenuItem>
        <DropdownMenuItem onClick={onRecheck}>
          <ArrowsClockwiseIcon />
          {t('page.row.recheck')}
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => {
            copy(domain.domain)
          }}
        >
          {copied ? <CheckIcon /> : <CopyIcon />}
          {copied ? t('page.row.copied') : t('page.row.copy')}
        </DropdownMenuItem>
        <DropdownMenuItem
          render={
            <Link
              to="/apps/$name/domains"
              params={{ name: domain.service_name }}
            />
          }
        >
          <GearSixIcon />
          {t('page.row.manage')}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

// Plain query, no polling: one row per visible domain must stay cheap.
function useDomainDns(domain: Domain) {
  return useQuery({
    ...domainCheckQueryOptions(domainScope(domain), domain.domain),
    staleTime: 60_000,
  })
}

export function DomainNameCell({
  domain,
  cert: storedCert,
}: {
  domain: Domain
  cert?: CertificateStatus
}) {
  const cert = effectiveDomainCert(domain, storedCert)
  const { t } = useTranslation('domains')
  const dns = useDomainDns(domain)
  const warning = inlineWarning(domain, cert, dns.data, t)
  const flags = FLAGS.filter((flag) => domain[flag.key])

  return (
    <div className="min-w-0">
      <div className="flex min-w-0 flex-wrap items-center gap-1.5">
        <Link
          to="/domains/$domain"
          params={{ domain: domain.domain }}
          search={{ app: domainScope(domain) }}
          className="min-w-0 font-mono text-sm font-medium break-all text-foreground hover:underline"
        >
          {domain.domain}
        </Link>
        <a
          href={`https://${domain.domain.replace(/^\*\./, '')}`}
          target="_blank"
          rel="noreferrer noopener"
          aria-label={t('page.row.open', { domain: domain.domain })}
          className="shrink-0 text-muted-foreground hover:text-foreground"
        >
          <ArrowSquareOutIcon className="size-3" aria-hidden="true" />
        </a>
        {domain.automatic ? (
          <Badge variant="outline">{t('page.row.automatic')}</Badge>
        ) : null}
        {domain.domain.startsWith('*.') ? (
          <Badge variant="outline" title={t('page.row.wildcardHint')}>
            {t('page.row.wildcard')}
          </Badge>
        ) : null}
        {flags.map((flag) => {
          const enforced = isEnforced(domain.enforcement, flag.control)
          const label = enforced
            ? t(flag.label)
            : t('enforcement.flagTitle', { label: t(flag.label) })
          return (
            <span
              key={flag.key}
              className={
                enforced
                  ? 'shrink-0 text-muted-foreground'
                  : 'shrink-0 text-tone-warning'
              }
              title={label}
            >
              <flag.icon className="size-3.5" aria-hidden="true" />
              <span className="sr-only">{label}</span>
            </span>
          )
        })}
      </div>
      {warning ? (
        <p
          className="mt-0.5 flex min-w-0 items-start gap-1 text-xs text-amber-700 dark:text-amber-400"
          title={warning.title}
        >
          <WarningCircleIcon
            className="mt-0.5 size-3.5 shrink-0"
            aria-hidden="true"
          />
          <span>{warning.text}</span>
        </p>
      ) : null}
      {domain.issues?.[0] ? (
        <DomainIssueFix domain={domain} issue={domain.issues[0]} />
      ) : null}
    </div>
  )
}

export function DomainTargetCell({ domain }: { domain: Domain }) {
  if (isStandaloneDomain(domain))
    return <StandaloneTargetCell domain={domain} />
  return (
    <Link
      to="/apps/$name"
      params={{ name: domain.service_name }}
      className="block min-w-0 truncate text-sm text-muted-foreground hover:text-foreground hover:underline"
    >
      {domain.service_name}
    </Link>
  )
}

export function DomainDnsCell({ domain }: { domain: Domain }) {
  const dns = useDomainDns(domain)
  return <DomainDnsStatusBadge status={dns.data?.status} />
}

export function DomainCertCell({
  domain,
  cert,
}: {
  domain: Domain
  cert?: CertificateStatus
}) {
  return <CertCell domain={domain} cert={effectiveDomainCert(domain, cert)} />
}

export function DomainExpiryCell({
  domain,
  cert,
}: {
  domain: Domain
  cert?: CertificateStatus
}) {
  return <ExpiryCell cert={effectiveDomainCert(domain, cert)} />
}

export { SearchVisibilityCell as DomainVisibilityCell }

export function DomainRowActions({ domain }: { domain: Domain }) {
  const [recordOpen, setRecordOpen] = useState(false)
  const [diagnoseOpen, setDiagnoseOpen] = useState(false)
  const dns = useDomainDns(domain)
  const onShowRecord = () => {
    setRecordOpen(true)
  }
  const onDiagnose = () => {
    setDiagnoseOpen(true)
  }
  const onRecheck = () => {
    void dns.refetch()
  }

  return (
    <>
      {isStandaloneDomain(domain) ? (
        <StandaloneRowActions
          domain={domain}
          onShowRecord={onShowRecord}
          onRecheck={onRecheck}
          onDiagnose={onDiagnose}
        />
      ) : (
        <RowActions
          domain={domain}
          onShowRecord={onShowRecord}
          onRecheck={onRecheck}
          onDiagnose={onDiagnose}
        />
      )}
      {diagnoseOpen ? (
        <DomainDiagnoseDialog
          domain={domain.domain}
          onClose={() => {
            setDiagnoseOpen(false)
          }}
        />
      ) : null}
      {recordOpen ? (
        <DomainDnsRecordDialog
          appName={domainScope(domain)}
          domain={domain.domain}
          onClose={() => {
            setRecordOpen(false)
          }}
        />
      ) : null}
    </>
  )
}
