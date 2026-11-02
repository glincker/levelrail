import { useEffect, useState } from 'react'
import {
  CheckGlassIcon,
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  CopyGlassIcon,
  RefreshCwGlassIcon,
  type UiIcon,
} from '@thesvg/ui-react'
import type { VariantProps } from 'class-variance-authority'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useTranslation } from 'react-i18next'
import { useCopyToClipboard } from '../hooks/useCopyToClipboard'
import {
  useDomainCheck,
  type AcmeFailure,
  type DnsHint,
  type DnsProvider,
  type DnsSource,
  type DnsVerdict,
  type DomainCheckStatus,
  type DomainResolver,
  type ProxyCertificate,
  type TlsOwner,
} from '../queries/domainCheck'
import {
  dnsCheckAnchor,
  dnsPillState,
  proxyCertTrusted,
  type DnsPillState,
} from '../lib/dnsPill'
import { findProxyDomain } from '../lib/proxyIntegration'
import {
  proxyIntegrationQueryOptions,
  type ProxyDomain,
} from '../queries/proxyIntegration'
import { useQuery } from '@tanstack/react-query'
import { AcmeFailureHint } from './AcmeFailureHint'
import { DnsProviderBadge } from './DnsProviderBadge'
import { ProxyManagedCertNote } from './ProxyManagedCertNote'

// The subset of a domain-check result DomainCheckPanel needs to render:
// shared by domainCheck.ts's per-app DomainCheckResult and domains.ts's
// platform-level IngressDomainCheckResult, so one panel serves both
// without either query module depending on the other's types.
export interface DomainCheckPanelData {
  expected_host?: string
  expected_ipv4?: string[]
  expected_ipv6?: string[]
  host_inferred?: boolean
  status: DomainCheckStatus
  expected_private?: boolean
  dns_provider?: DnsProvider
  acme_failure?: AcmeFailure
  tls_owner?: TlsOwner
  proxy_certificate?: ProxyCertificate
  checked_at?: string
  resolvers?: DomainResolver[]
  authoritative?: DnsSource[]
  public?: DnsSource[]
  local?: DnsSource[]
  verdict?: DnsVerdict
  local_stale?: boolean
  retry_at?: string
  escalated?: boolean
  flush_command?: string
  hints?: DnsHint[]
}

type DnsRecordType = 'A' | 'AAAA' | 'CNAME'

function dnsRecordType(host: string): DnsRecordType {
  if (host.includes(':')) return 'AAAA'
  if (/^\d{1,3}(\.\d{1,3}){3}$/.test(host)) return 'A'
  return 'CNAME'
}

interface DnsRecordOption {
  type: DnsRecordType
  value: string
}

// One option per record type the operator could actually add. A raw IP
// APP_PUBLIC_HOST has exactly one: its own family, nothing to choose
// between. A hostname APP_PUBLIC_HOST offers CNAME first (the simplest,
// works even if the server's IP changes later) plus a direct A/AAAA
// alternative for each family it resolves to, since some registrars
// reject CNAME at the domain apex.
function dnsRecordOptions(data: DomainCheckPanelData): DnsRecordOption[] {
  const host = data.expected_host
  if (!host) return []
  const type = dnsRecordType(host)
  if (type !== 'CNAME') return [{ type, value: host }]
  const options: DnsRecordOption[] = [{ type: 'CNAME', value: host }]
  if (data.expected_ipv4?.[0])
    options.push({ type: 'A', value: data.expected_ipv4[0] })
  if (data.expected_ipv6?.[0])
    options.push({ type: 'AAAA', value: data.expected_ipv6[0] })
  return options
}

// One row of the DNS record block below: a label, the value itself
// (monospace, selectable), and its own copy button. Split out since the
// record has two independently-copyable fields (Name and Value) and a
// registrar's DNS form is two separate input boxes, not one string to
// paste in one place.
function DnsRecordRow({ label, value }: { label: string; value: string }) {
  const { copied, copy } = useCopyToClipboard()
  return (
    <div className="flex items-center gap-2 rounded-md border border-input bg-background p-1.5">
      <span className="w-12 shrink-0 text-xs font-medium text-muted-foreground">
        {label}
      </span>
      <code className="min-w-0 flex-1 overflow-x-auto text-xs break-all">
        {value}
      </code>
      <Button
        type="button"
        size="icon-sm"
        variant="ghost"
        onClick={() => {
          copy(value)
        }}
        aria-label={copied ? `${label} copied` : `Copy ${label}`}
      >
        {copied ? (
          <CheckGlassIcon className="size-5" />
        ) : (
          <CopyGlassIcon className="size-5" />
        )}
      </Button>
    </div>
  )
}

const STATUS_META: Record<
  DnsPillState,
  {
    label: string
    variant: VariantProps<typeof badgeVariants>['variant']
    icon: UiIcon
  }
> = {
  connected: {
    label: 'Connected',
    variant: 'success',
    icon: CircleCheckGlassIcon,
  },
  propagating: {
    label: 'Propagating',
    variant: 'warning',
    icon: CircleAlertGlassIcon,
  },
  not_resolving: {
    label: 'Not resolving yet',
    variant: 'warning',
    icon: CircleAlertGlassIcon,
  },
  resolves_elsewhere: {
    label: 'Points elsewhere',
    variant: 'destructive',
    icon: CircleAlertGlassIcon,
  },
  unconfigured: {
    label: 'Unable to check',
    variant: 'muted',
    icon: CircleAlertGlassIcon,
  },
  cert_pending: {
    label: '',
    variant: 'warning',
    icon: CircleAlertGlassIcon,
  },
}

const VERDICT_VARIANT: Record<
  DnsVerdict,
  VariantProps<typeof badgeVariants>['variant']
> = {
  dns_ready: 'success',
  public_ready: 'warning',
  not_found: 'warning',
  points_elsewhere: 'destructive',
  unknown: 'muted',
}

// Hints whose text has no per-domain values are translated here; the rest
// (addresses, name servers) show the server's own sentence.
const STATIC_HINTS = new Set([
  'local_stale',
  'local_stale_long',
  'was_cloudflare_proxy',
  'waiting',
  'check_zone',
  'check_aaaa',
  'check_cname',
  'check_nameservers',
])

function useRemainingMs(iso?: string): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!iso) return undefined
    const id = setInterval(() => {
      setNow(Date.now())
    }, 1000)
    return () => {
      clearInterval(id)
    }
  }, [iso])
  return iso ? Math.max(0, Date.parse(iso) - now) : 0
}

function formatCountdown(ms: number): string {
  const total = Math.ceil(ms / 1000)
  const m = Math.floor(total / 60)
  const sec = String(total % 60).padStart(2, '0')
  return `${m}:${sec}`
}

function DnsAnswerGroup({
  label,
  sources,
}: {
  label: string
  sources?: DnsSource[]
}) {
  const { t } = useTranslation('domains')
  if (!sources || sources.length === 0) return null
  return (
    <div>
      <div className="font-medium text-foreground">{label}</div>
      <ul className="space-y-0.5 text-muted-foreground">
        {sources.map((r) => (
          <li key={r.name} className="flex gap-2">
            <span className="w-32 shrink-0 truncate">{r.name}</span>
            <span className="min-w-0 break-all font-mono">
              {r.addresses && r.addresses.length > 0
                ? r.addresses.join(', ')
                : (r.error ?? t('dnsCheck.noAnswer'))}
              {r.ttl ? (
                <span className="ml-1 text-muted-foreground/70">
                  {t('dnsCheck.ttl', { seconds: r.ttl })}
                </span>
              ) : null}
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}

function FlushCommand({ command }: { command: string }) {
  const { t } = useTranslation('domains')
  const { copied, copy } = useCopyToClipboard()
  return (
    <div className="space-y-1">
      <p className="text-muted-foreground">{t('dnsCheck.flushIntro')}</p>
      <div className="flex items-center gap-2 rounded-md border border-input bg-background p-1.5">
        <code className="min-w-0 flex-1 overflow-x-auto text-xs">
          {command}
        </code>
        <Button
          type="button"
          size="icon-sm"
          variant="ghost"
          onClick={() => {
            copy(command)
          }}
          aria-label={t('dnsCheck.copyFlush')}
        >
          {copied ? (
            <CheckGlassIcon className="size-5" />
          ) : (
            <CopyGlassIcon className="size-5" />
          )}
        </Button>
      </div>
    </div>
  )
}

// What the zone's own servers, public resolvers and this server each
// answered, the verdict, the cache countdown and what to check next, so
// "Check now" always ends with an explanation instead of an unchanged badge.
function DnsResolverReport({ data }: { data: DomainCheckPanelData }) {
  const { t } = useTranslation('domains')
  const remaining = useRemainingMs(data.retry_at)
  if (data.verdict) {
    const variant = VERDICT_VARIANT[data.verdict]
    return (
      <div className="mt-2 space-y-2 text-xs">
        <Badge variant={variant}>{t(`dnsCheck.verdict.${data.verdict}`)}</Badge>
        <DnsAnswerGroup
          label={t('dnsCheck.groups.authoritative')}
          sources={data.authoritative}
        />
        <DnsAnswerGroup
          label={t('dnsCheck.groups.public')}
          sources={data.public}
        />
        <DnsAnswerGroup
          label={t('dnsCheck.groups.local')}
          sources={data.local}
        />
        {data.local_stale && data.retry_at ? (
          <p className="text-foreground" aria-live="off">
            {t('dnsCheck.retryAt', {
              time: new Date(data.retry_at).toLocaleTimeString([], {
                hour: '2-digit',
                minute: '2-digit',
              }),
            })}
            {remaining > 0
              ? ` ${t('dnsCheck.countdown', { left: formatCountdown(remaining) })}`
              : ''}
          </p>
        ) : null}
        {data.local_stale && data.flush_command ? (
          <FlushCommand command={data.flush_command} />
        ) : null}
        {data.hints && data.hints.length > 0 ? (
          <ul className="list-disc space-y-0.5 pl-4 text-foreground">
            {data.hints.map((h) => (
              <li key={h.code}>
                {STATIC_HINTS.has(h.code)
                  ? t(`dnsCheck.hints.${h.code}`, { defaultValue: h.message })
                  : h.message}
              </li>
            ))}
          </ul>
        ) : null}
      </div>
    )
  }
  const resolvers = data.resolvers ?? []
  if (resolvers.length === 0) return null
  return (
    <div className="mt-2 space-y-1 text-xs">
      <ul className="space-y-0.5 text-muted-foreground">
        {resolvers.map((r) => (
          <li key={r.name} className="flex gap-2">
            <span className="w-24 shrink-0">{r.name}</span>
            <span className="font-mono">
              {r.addresses && r.addresses.length > 0
                ? r.addresses.join(', ')
                : (r.error ?? t('dnsCheck.noAnswer'))}
            </span>
          </li>
        ))}
      </ul>
      <p className="text-foreground">
        {data.status === 'propagating'
          ? t('dnsCheck.propagatingHelp')
          : data.status === 'not_resolving'
            ? t('dnsCheck.notFoundHelp')
            : null}
      </p>
    </div>
  )
}

// The rendering half of the DNS-check guidance layer: the exact record
// to add and a status badge, driven entirely by props so both the
// per-app check (DomainDnsCheck below) and the platform-level primary
// domain check (IngressSettingsCard) render it identically without
// duplicating this markup.
export function DomainCheckPanel({
  domain,
  data,
  isFetching,
  onRefetch,
  proxyRow,
}: {
  domain: string
  data?: DomainCheckPanelData
  isFetching: boolean
  onRefetch: () => void
  proxyRow?: ProxyDomain
}) {
  const { t } = useTranslation('domains')
  const proxyOwned = data?.tls_owner === 'proxy'
  const pill = data
    ? dnsPillState(
        data.status,
        proxyOwned,
        proxyCertTrusted(proxyRow, data.proxy_certificate),
      )
    : undefined
  const meta = pill ? STATUS_META[pill] : undefined
  const StatusIcon = meta?.icon
  const options = data ? dnsRecordOptions(data) : []
  const [activeType, setActiveType] = useState<DnsRecordType | undefined>(
    options[0]?.type,
  )
  const active = options.find((o) => o.type === activeType) ?? options[0]

  return (
    <div
      id={dnsCheckAnchor(domain)}
      className="rounded-md border border-border bg-muted/30 p-3 text-sm"
    >
      <div className="flex items-center justify-between gap-2">
        <span className="min-w-0 truncate font-mono text-xs text-muted-foreground">
          {domain}
        </span>
        {meta ? (
          <Badge variant={meta.variant} className="shrink-0">
            {StatusIcon ? <StatusIcon className="size-3.5" /> : null}
            {pill === 'cert_pending' ? t('dnsCheck.certPending') : meta.label}
          </Badge>
        ) : null}
      </div>

      {data && data.status !== 'connected' && active ? (
        <div className="mt-2 space-y-1.5">
          <p className="text-xs text-muted-foreground">
            Add {options.length > 1 ? 'one of these records' : 'this record'} at
            your registrar:
            {data.status === 'resolves_elsewhere'
              ? ' it currently resolves somewhere else, double check the record you added.'
              : ''}
          </p>
          {options.length > 1 ? (
            <Tabs
              value={active.type}
              onValueChange={(v) => {
                setActiveType(v as DnsRecordType)
              }}
            >
              <TabsList>
                {options.map((o) => (
                  <TabsTrigger key={o.type} value={o.type}>
                    {o.type}
                  </TabsTrigger>
                ))}
              </TabsList>
              {options.map((o) => (
                <TabsContent
                  key={o.type}
                  value={o.type}
                  className="space-y-1 pt-2"
                >
                  <DnsRecordRow label="Name" value={domain} />
                  <DnsRecordRow label="Value" value={o.value} />
                </TabsContent>
              ))}
            </Tabs>
          ) : (
            <div className="space-y-1">
              <DnsRecordRow label="Type" value={active.type} />
              <DnsRecordRow label="Name" value={domain} />
              <DnsRecordRow label="Value" value={active.value} />
            </div>
          )}
        </div>
      ) : null}

      {/* Shown regardless of status, including (especially) "connected":
          a green checkmark built on a guessed host is exactly the case
          where the operator most needs to know it isn't a confirmed
          address, not just when they're still troubleshooting. */}
      {data?.host_inferred ? (
        <p className="mt-2 text-xs text-muted-foreground">
          {data.status === 'connected' ? 'This is a ' : 'This is only a '}
          best guess based on how you reached this dashboard, not a confirmed
          address. Set <code className="font-mono">APP_PUBLIC_HOST</code> on the
          control plane to check against a confirmed one instead.
        </p>
      ) : null}

      {proxyOwned && !proxyRow ? (
        <div className="mt-2">
          <ProxyManagedCertNote certificate={data?.proxy_certificate} />
        </div>
      ) : null}
      {data?.expected_private && !proxyOwned ? (
        <p className="mt-2 text-xs text-destructive">
          {t('certificate.dns01Private', { host: data.expected_host ?? '' })}
        </p>
      ) : null}
      {data?.dns_provider && !proxyOwned ? (
        <div className="mt-2">
          <DnsProviderBadge provider={data.dns_provider} />
        </div>
      ) : null}
      {data?.acme_failure && !proxyOwned ? (
        <div className="mt-2">
          <AcmeFailureHint failure={data.acme_failure} />
        </div>
      ) : null}

      {data && (data.status !== 'connected' || data.local_stale) ? (
        <DnsResolverReport data={data} />
      ) : null}

      <div className="mt-2 flex items-center gap-2" aria-live="polite">
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={onRefetch}
          disabled={isFetching}
        >
          <RefreshCwGlassIcon
            className={isFetching ? 'size-5 animate-spin' : 'size-5'}
          />
          {isFetching ? t('dnsCheck.checking') : 'Check now'}
        </Button>
        {data?.checked_at && !isFetching ? (
          <span className="text-xs text-muted-foreground">
            {t('dnsCheck.checkedAt', {
              time: new Date(data.checked_at).toLocaleTimeString(),
            })}
          </span>
        ) : null}
      </div>
    </div>
  )
}

// Sits under one saved domain in DomainEditor: the guidance layer this
// project's gap analysis called out, an operator who doesn't already
// know what an A record is has no idea what to actually go do after
// typing a domain name. Polls GET
// /api/v1/apps/{name}/domains/{domain}/check (bounded, see
// queries/domainCheck.ts's own MAX_AUTO_POLL_ATTEMPTS comment) until it
// resolves, with a manual "Check now" for whenever the auto-poll has
// already given up or an operator wants an immediate answer.
export function DomainDnsCheck({
  appName,
  domain,
}: {
  appName: string
  domain: string
}) {
  const { data, isFetching, refetch } = useDomainCheck(appName, domain)
  const { data: proxy } = useQuery(proxyIntegrationQueryOptions())
  const proxyRow = proxy ? findProxyDomain(proxy, domain) : undefined
  return (
    <DomainCheckPanel
      proxyRow={proxyRow}
      domain={domain}
      data={data}
      isFetching={isFetching}
      onRefetch={() => {
        void refetch()
      }}
    />
  )
}
