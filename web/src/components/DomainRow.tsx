import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  ClockGlassIcon,
  CopyGlassIcon,
  EllipsisGlassIcon,
  ExternalLinkGlassIcon,
  FileSearchGlassIcon,
  LockGlassIcon,
  RefreshCwGlassIcon,
  ArrowRightGlassIcon,
  SettingsGlassIcon,
  ShieldCheckGlassIcon,
  StarGlassIcon,
  TrashGlassIcon,
  UnlinkGlassIcon,
  SignpostGlassIcon,
  StethoscopeGlassIcon,
  WrenchGlassIcon,
  type UiIcon,
} from '@thesvg/ui-react'
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
import { cn } from '../lib/utils'
import { TLS_TONE_CLASS, tlsDaysText, tlsTone } from '../lib/domainTls'
import { useCopyToClipboard } from '../hooks/useCopyToClipboard'
import { CertTruthBadge } from './CertTruthBadge'
import type { CertificateStatus } from '../queries/certificates'
import { domainCheckQueryOptions } from '../queries/domainCheck'
import type { DomainCheckResult } from '../queries/domainCheck'
import {
  useDomainSearchVisibility,
  useSetDomainSearchVisibility,
} from '../queries/domainSearchVisibility'
import {
  ingressSettingsQueryOptions,
  useUpdateIngressSettings,
  type Domain,
} from '../queries/domains'
import { canParkOrMove, isPrimaryDomain } from '../lib/domainLifecycle'
import {
  DeleteDomainDialog,
  MoveDomainDialog,
  ParkDomainDialog,
} from './domains/DomainLifecycleDialogs'
import { domainScope, isStandaloneDomain } from '../lib/standaloneDomain'
import { isEnforced } from '../lib/domainEnforcement'
import type { TrafficControl } from '../lib/domainEnforcement'
import { DomainDnsRecordDialog } from './DomainDnsRecordDialog'
import { DomainIssueFix } from './DomainIssueFix'
import { DomainDnsStatusBadge } from './DomainDnsStatusBadge'
import { DomainWatchChip } from './domains/DomainWatchChip'
import { DomainDiagnoseDialog } from './domains/DomainDiagnoseDialog'
import {
  StandaloneRowActions,
  StandaloneTargetCell,
} from './domains/StandaloneRowParts'
import { CopyAsCommand } from './CopyAsCommand'

const FLAGS: {
  key: keyof Pick<
    Domain,
    'waf_enabled' | 'has_redirect' | 'maintenance_enabled' | 'has_basic_auth'
  >
  control: TrafficControl
  icon: UiIcon
  label:
    | 'page.flag.waf'
    | 'page.flag.redirect'
    | 'page.flag.maintenance'
    | 'page.flag.basicAuth'
}[] = [
  {
    key: 'waf_enabled',
    control: 'waf',
    icon: ShieldCheckGlassIcon,
    label: 'page.flag.waf',
  },
  {
    key: 'has_redirect',
    control: 'redirect',
    icon: SignpostGlassIcon,
    label: 'page.flag.redirect',
  },
  {
    key: 'maintenance_enabled',
    control: 'maintenance',
    icon: WrenchGlassIcon,
    label: 'page.flag.maintenance',
  },
  {
    key: 'has_basic_auth',
    control: 'basic_auth',
    icon: LockGlassIcon,
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
  const renewal = cert ? certRenewalBadge(cert) : null
  // One chip per row; fixing the certificate lives on the domain page.
  if (renewal) {
    return (
      <Badge variant="destructive" title={renewal.hint}>
        {renewal.label}
      </Badge>
    )
  }
  if (cert) return <CertTruthBadge cert={cert} />
  return domain.acme_failure ? (
    <Badge variant="destructive">{t('page.cert.failed')}</Badge>
  ) : (
    <span className="text-xs text-muted-foreground/70">
      {t('page.cert.none')}
    </span>
  )
}

function TlsDaysLine({ daysLeft }: { daysLeft: number }) {
  const { t } = useTranslation('domains')
  const { key, count } = tlsDaysText(daysLeft)
  return (
    <span className={cn('truncate', TLS_TONE_CLASS[tlsTone(daysLeft)])}>
      {t(key, { count })}
    </span>
  )
}

function ExpiryCell({
  cert,
  daysLeft,
}: {
  cert?: CertificateStatus
  daysLeft?: number
}) {
  const { t } = useTranslation('domains')
  if (!cert && daysLeft !== undefined) {
    return (
      <span className="flex min-w-0 flex-col text-xs">
        <TlsDaysLine daysLeft={daysLeft} />
      </span>
    )
  }
  if (!cert) return <span className="text-xs text-muted-foreground/60">-</span>
  if (isInternalCert(cert)) {
    return <span className="text-xs text-muted-foreground/60">-</span>
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
      {daysLeft !== undefined ? (
        <TlsDaysLine daysLeft={daysLeft} />
      ) : (
        <span className="truncate">{certExpiryLabel(cert.not_after)}</span>
      )}
      <span className="truncate text-muted-foreground/70">{renews}</span>
    </span>
  )
}

type Lifecycle = 'park' | 'move' | 'delete'

function RowActions({
  domain,
  onShowRecord,
  onRecheck,
  onDiagnose,
  onLifecycle,
}: {
  domain: Domain
  onShowRecord: () => void
  onRecheck: () => void
  onDiagnose: () => void
  onLifecycle: (kind: Lifecycle) => void
}) {
  const { t } = useTranslation('domains')
  const { copied, copy } = useCopyToClipboard()
  const { data: ingress } = useQuery(ingressSettingsQueryOptions())
  const setPrimary = useUpdateIngressSettings()
  const primary = isPrimaryDomain(domain.domain, ingress?.primary_domain)
  const manageable = canParkOrMove(domain)
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
        <EllipsisGlassIcon size="1.4em" />
      </DropdownMenuTrigger>
      <DropdownMenuContent>
        <DropdownMenuItem
          render={
            <Link
              to="/apps/$name/domains/$domain"
              params={{ name: domainScope(domain), domain: domain.domain }}
            />
          }
        >
          <SettingsGlassIcon size="1.4em" />
          {t('page.row.details')}
        </DropdownMenuItem>
        <DropdownMenuItem onClick={onShowRecord}>
          <FileSearchGlassIcon size="1.4em" />
          {t('page.row.dnsRecord')}
        </DropdownMenuItem>
        <DropdownMenuItem onClick={onDiagnose}>
          <StethoscopeGlassIcon size="1.4em" />
          {t('page.row.diagnose')}
        </DropdownMenuItem>
        <DropdownMenuItem onClick={onRecheck}>
          <RefreshCwGlassIcon size="1.4em" />
          {t('page.row.recheck')}
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => {
            copy(domain.domain)
          }}
        >
          {copied ? (
            <CheckGlassIcon size="1.4em" />
          ) : (
            <CopyGlassIcon size="1.4em" />
          )}
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
          <SettingsGlassIcon size="1.4em" />
          {t('page.row.manage')}
        </DropdownMenuItem>
        {ingress && !primary ? (
          <DropdownMenuItem
            disabled={setPrimary.isPending}
            onClick={() => {
              setPrimary.mutate({ ...ingress, primary_domain: domain.domain })
            }}
          >
            <StarGlassIcon size="1.4em" />
            {t('page.row.setPrimary')}
          </DropdownMenuItem>
        ) : null}
        {manageable ? (
          <>
            <DropdownMenuItem
              onClick={() => {
                onLifecycle('move')
              }}
            >
              <ArrowRightGlassIcon size="1.4em" />
              {t('page.row.move')}
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={() => {
                onLifecycle('park')
              }}
            >
              <UnlinkGlassIcon size="1.4em" />
              {t('page.row.park')}
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={() => {
                onLifecycle('delete')
              }}
            >
              <TrashGlassIcon size="1.4em" />
              {t('page.row.delete')}
            </DropdownMenuItem>
          </>
        ) : null}
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

const NAME_LINK =
  'min-w-0 truncate font-mono text-sm font-medium text-foreground hover:underline'

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
  const { data: ingress } = useQuery(ingressSettingsQueryOptions())
  const isPrimary = isPrimaryDomain(domain.domain, ingress?.primary_domain)
  const warning = inlineWarning(domain, cert, dns.data, t)
  const flags = FLAGS.filter((flag) => domain[flag.key])

  return (
    <div className="min-w-0">
      <div className="flex min-w-0 flex-wrap items-center gap-1.5">
        {isStandaloneDomain(domain) ? (
          <Link
            to="/domains/$domain"
            params={{ domain: domain.domain }}
            search={{ app: domainScope(domain) }}
            title={domain.domain}
            className={NAME_LINK}
          >
            {domain.domain}
          </Link>
        ) : (
          <Link
            to="/apps/$name/domains/$domain"
            params={{ name: domainScope(domain), domain: domain.domain }}
            title={domain.domain}
            className={NAME_LINK}
          >
            {domain.domain}
          </Link>
        )}
        <a
          href={`https://${domain.domain.replace(/^\*\./, '')}`}
          target="_blank"
          rel="noreferrer noopener"
          aria-label={t('page.row.open', { domain: domain.domain })}
          className="shrink-0 text-muted-foreground hover:text-foreground"
        >
          <ExternalLinkGlassIcon className="size-4.5" aria-hidden="true" />
        </a>
        {isPrimary ? (
          <Badge variant="info">{t('page.row.primary')}</Badge>
        ) : null}
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
              <flag.icon className="size-4.5" aria-hidden="true" />
              <span className="sr-only">{label}</span>
            </span>
          )
        })}
      </div>
      {warning ? (
        <p
          className="mt-0.5 flex min-w-0 items-start gap-1 text-xs text-muted-foreground"
          title={warning.title}
        >
          <ClockGlassIcon
            className="mt-0.5 size-4.5 shrink-0 text-tone-warning"
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
  return (
    <div className="flex flex-wrap items-center gap-1">
      <DomainDnsStatusBadge status={dns.data?.status} />
      <DomainWatchChip domain={domain.domain} />
    </div>
  )
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
  return (
    <ExpiryCell
      cert={effectiveDomainCert(domain, cert)}
      daysLeft={domain.tls_days_left}
    />
  )
}

export function DomainEnvironmentCell({ domain }: { domain: Domain }) {
  if (!domain.environment) {
    return <span className="text-xs text-muted-foreground/60">-</span>
  }
  return (
    <span className="block truncate text-xs" title={domain.environment}>
      {domain.environment}
    </span>
  )
}

export { SearchVisibilityCell as DomainVisibilityCell }

export function DomainRowActions({ domain }: { domain: Domain }) {
  const [recordOpen, setRecordOpen] = useState(false)
  const [diagnoseOpen, setDiagnoseOpen] = useState(false)
  const [lifecycle, setLifecycle] = useState<Lifecycle | null>(null)
  const dns = useDomainDns(domain)
  const closeLifecycle = (open: boolean) => {
    if (!open) setLifecycle(null)
  }
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
        <span className="flex items-center gap-1 justify-self-end">
          <CopyAsCommand
            subject={domain.domain}
            actions={[
              {
                kind: 'domain.check',
                app: domain.service_name,
                domain: domain.domain,
              },
              {
                kind: 'cert.renew',
                app: domain.service_name,
                domain: domain.domain,
              },
              {
                kind: 'domain.remove',
                app: domain.service_name,
                domain: domain.domain,
              },
            ]}
          />
          <RowActions
            domain={domain}
            onShowRecord={onShowRecord}
            onRecheck={onRecheck}
            onDiagnose={onDiagnose}
            onLifecycle={setLifecycle}
          />
        </span>
      )}
      {lifecycle === 'park' ? (
        <ParkDomainDialog
          app={domain.service_name}
          domain={domain.domain}
          open
          onOpenChange={closeLifecycle}
        />
      ) : null}
      {lifecycle === 'move' ? (
        <MoveDomainDialog
          from={domain.service_name}
          domain={domain.domain}
          open
          onOpenChange={closeLifecycle}
        />
      ) : null}
      {lifecycle === 'delete' ? (
        <DeleteDomainDialog
          app={domain.service_name}
          domain={domain.domain}
          open
          onOpenChange={closeLifecycle}
        />
      ) : null}
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
