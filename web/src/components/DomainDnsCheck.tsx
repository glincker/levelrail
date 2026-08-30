import { useState } from 'react'
import {
  ArrowsClockwiseIcon,
  CheckCircleIcon,
  CheckIcon,
  CopyIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import type { VariantProps } from 'class-variance-authority'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useCopyToClipboard } from '../hooks/useCopyToClipboard'
import { useDomainCheck, type DomainCheckStatus } from '../queries/domainCheck'

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
  if (data.expected_ipv4?.[0]) options.push({ type: 'A', value: data.expected_ipv4[0] })
  if (data.expected_ipv6?.[0]) options.push({ type: 'AAAA', value: data.expected_ipv6[0] })
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
          <CheckIcon className="size-3.5" />
        ) : (
          <CopyIcon className="size-3.5" />
        )}
      </Button>
    </div>
  )
}

const STATUS_META: Record<
  DomainCheckStatus,
  {
    label: string
    variant: VariantProps<typeof badgeVariants>['variant']
    icon: Icon
  }
> = {
  connected: { label: 'Connected', variant: 'success', icon: CheckCircleIcon },
  not_resolving: {
    label: 'Not resolving yet',
    variant: 'warning',
    icon: WarningCircleIcon,
  },
  resolves_elsewhere: {
    label: 'Points elsewhere',
    variant: 'destructive',
    icon: WarningCircleIcon,
  },
  unconfigured: {
    label: 'Unable to check',
    variant: 'muted',
    icon: WarningCircleIcon,
  },
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
}: {
  domain: string
  data?: DomainCheckPanelData
  isFetching: boolean
  onRefetch: () => void
}) {
  const meta = data ? STATUS_META[data.status] : undefined
  const StatusIcon = meta?.icon
  const options = data ? dnsRecordOptions(data) : []
  const [activeType, setActiveType] = useState<DnsRecordType | undefined>(options[0]?.type)
  const active = options.find((o) => o.type === activeType) ?? options[0]

  return (
    <div className="rounded-md border border-border bg-muted/30 p-3 text-sm">
      <div className="flex items-center justify-between gap-2">
        <span className="min-w-0 truncate font-mono text-xs text-muted-foreground">
          {domain}
        </span>
        {meta ? (
          <Badge variant={meta.variant} className="shrink-0">
            {StatusIcon ? <StatusIcon className="size-3" /> : null}
            {meta.label}
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
            <Tabs value={active.type} onValueChange={(v) => { setActiveType(v as DnsRecordType) }}>
              <TabsList>
                {options.map((o) => (
                  <TabsTrigger key={o.type} value={o.type}>
                    {o.type}
                  </TabsTrigger>
                ))}
              </TabsList>
              {options.map((o) => (
                <TabsContent key={o.type} value={o.type} className="space-y-1 pt-2">
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
          best guess based on how you reached this dashboard, not a
          confirmed address. Set <code className="font-mono">APP_PUBLIC_HOST</code>{' '}
          on the control plane to check against a confirmed one instead.
        </p>
      ) : null}

      <div className="mt-2 flex items-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={onRefetch}
          disabled={isFetching}
        >
          <ArrowsClockwiseIcon
            className={isFetching ? 'size-3.5 animate-spin' : 'size-3.5'}
          />
          Check now
        </Button>
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
  return (
    <DomainCheckPanel
      domain={domain}
      data={data}
      isFetching={isFetching}
      onRefetch={() => {
        void refetch()
      }}
    />
  )
}
