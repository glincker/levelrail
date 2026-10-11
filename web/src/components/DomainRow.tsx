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
  certExpiryLabel,
  certRenewalBadge,
  certStatusMeta,
  isInternalCert,
} from '../lib/certStatus'
import { useCopyToClipboard } from '../hooks/useCopyToClipboard'
import type { CertificateStatus } from '../queries/certificates'
import { domainCheckQueryOptions } from '../queries/domainCheck'
import type { DomainCheckResult } from '../queries/domainCheck'
import {
  useDomainSearchVisibility,
  useSetDomainSearchVisibility,
} from '../queries/domainSearchVisibility'
import type { Domain } from '../queries/domains'
import { DomainDnsRecordDialog } from './DomainDnsRecordDialog'
import { DomainDnsStatusBadge } from './DomainDnsStatusBadge'

// Shared by the sticky header in DomainsTable and every row, so labels
// line up with row content.
export const DOMAIN_LIST_GRID =
  'grid min-w-[58rem] grid-cols-[minmax(0,1.7fr)_minmax(0,1fr)_7rem_8rem_7rem_4.5rem_2rem] items-center gap-3'

const FLAGS: {
  key: keyof Pick<
    Domain,
    'waf_enabled' | 'has_redirect' | 'maintenance_enabled' | 'has_basic_auth'
  >
  icon: Icon
  label:
    | 'page.flag.waf'
    | 'page.flag.redirect'
    | 'page.flag.maintenance'
    | 'page.flag.basicAuth'
}[] = [
  { key: 'waf_enabled', icon: ShieldCheckIcon, label: 'page.flag.waf' },
  { key: 'has_redirect', icon: SignpostIcon, label: 'page.flag.redirect' },
  {
    key: 'maintenance_enabled',
    icon: WrenchIcon,
    label: 'page.flag.maintenance',
  },
  { key: 'has_basic_auth', icon: LockKeyIcon, label: 'page.flag.basicAuth' },
]

export function RowSkeleton() {
  return (
    <div
      className={`${DOMAIN_LIST_GRID} border-b border-border px-4 py-3`}
      aria-hidden="true"
    >
      <Skeleton className="h-4 w-44" />
      <Skeleton className="h-4 w-24" />
      <Skeleton className="h-5 w-16" />
      <Skeleton className="h-5 w-16" />
      <Skeleton className="h-4 w-16" />
      <Skeleton className="h-5 w-9" />
      <span />
    </div>
  )
}

// First problem wins, so a row carries at most one inline warning.
function inlineWarning(
  domain: Domain,
  cert: CertificateStatus | undefined,
  dns: DomainCheckResult | undefined,
  t: TFunction<'domains'>,
): { text: string; title: string } | null {
  const failure = cert?.acme_failure ?? domain.acme_failure
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
  if (dns.expected_private) {
    const text = t('page.dns.privateHint')
    return { text, title: text }
  }
  return null
}

function SearchVisibilityCell({ domain }: { domain: Domain }) {
  const { t } = useTranslation('domains')
  const { data, isLoading } = useDomainSearchVisibility(
    domain.service_name,
    domain.domain,
  )
  const set = useSetDomainSearchVisibility(domain.service_name, domain.domain)
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
  if (!cert) {
    return domain.acme_failure ? (
      <Badge variant="destructive">{t('page.cert.failed')}</Badge>
    ) : (
      <span className="text-xs text-muted-foreground/70">
        {t('page.cert.none')}
      </span>
    )
  }
  const renewal = certRenewalBadge(cert)
  return (
    <span className="flex min-w-0 flex-col items-start gap-0.5">
      <Badge variant={certStatusMeta[cert.status].variant}>
        {certStatusMeta[cert.status].label}
      </Badge>
      {renewal ? (
        <Badge variant="destructive" title={renewal.hint}>
          {renewal.label}
        </Badge>
      ) : null}
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
  return (
    <span
      className="truncate text-xs text-muted-foreground"
      title={cert.not_after}
    >
      {certExpiryLabel(cert.not_after)}
    </span>
  )
}

function RowActions({
  domain,
  onShowRecord,
  onRecheck,
}: {
  domain: Domain
  onShowRecord: () => void
  onRecheck: () => void
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

export function DomainRow({
  domain,
  cert,
}: {
  domain: Domain
  cert?: CertificateStatus
}) {
  const { t } = useTranslation('domains')
  const [recordOpen, setRecordOpen] = useState(false)
  // Plain query, no polling: one row per visible domain must stay cheap.
  const dns = useQuery({
    ...domainCheckQueryOptions(domain.service_name, domain.domain),
    staleTime: 60_000,
  })
  const warning = inlineWarning(domain, cert, dns.data, t)
  const flags = FLAGS.filter((flag) => domain[flag.key])

  return (
    <div
      className={`${DOMAIN_LIST_GRID} h-full w-full border-b border-border px-4 py-2 transition-colors hover:bg-muted/40`}
    >
      <div className="min-w-0">
        <div className="flex min-w-0 items-center gap-1.5">
          <Link
            to="/domains/$domain"
            params={{ domain: domain.domain }}
            search={{ app: domain.service_name }}
            className="min-w-0 truncate font-mono text-sm font-medium text-foreground hover:underline"
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
          {flags.map((flag) => (
            <span
              key={flag.key}
              className="shrink-0 text-muted-foreground"
              title={t(flag.label)}
            >
              <flag.icon className="size-3.5" aria-hidden="true" />
              <span className="sr-only">{t(flag.label)}</span>
            </span>
          ))}
        </div>
        {warning ? (
          <p
            className="mt-0.5 flex min-w-0 items-center gap-1 text-xs text-amber-700 dark:text-amber-400"
            title={warning.title}
          >
            <WarningCircleIcon
              className="size-3.5 shrink-0"
              aria-hidden="true"
            />
            <span className="truncate">{warning.text}</span>
          </p>
        ) : null}
      </div>

      <Link
        to="/apps/$name"
        params={{ name: domain.service_name }}
        className="min-w-0 truncate text-sm text-muted-foreground hover:text-foreground hover:underline"
      >
        {domain.service_name}
      </Link>

      <DomainDnsStatusBadge status={dns.data?.status} />

      <CertCell domain={domain} cert={cert} />

      <ExpiryCell cert={cert} />

      <SearchVisibilityCell domain={domain} />

      <RowActions
        domain={domain}
        onShowRecord={() => {
          setRecordOpen(true)
        }}
        onRecheck={() => {
          void dns.refetch()
        }}
      />

      {recordOpen ? (
        <DomainDnsRecordDialog
          appName={domain.service_name}
          domain={domain.domain}
          onClose={() => {
            setRecordOpen(false)
          }}
        />
      ) : null}
    </div>
  )
}
