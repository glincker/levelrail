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
import { useCopyToClipboard } from '../hooks/useCopyToClipboard'
import { useDomainCheck, type DomainCheckStatus } from '../queries/domainCheck'

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

// Sits under one saved domain in DomainEditor: the guidance layer this
// project's gap analysis called out, an operator who doesn't already
// know what an A record is has no idea what to actually go do after
// typing a domain name. Shows the exact record to add and polls GET
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
  const meta = data ? STATUS_META[data.status] : undefined
  const StatusIcon = meta?.icon

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

      {data && data.status !== 'connected' && data.expected_host ? (
        <div className="mt-2 space-y-1.5">
          <p className="text-xs text-muted-foreground">
            Add this <span className="font-medium text-foreground">A record</span>{' '}
            at your registrar:
            {data.status === 'resolves_elsewhere'
              ? ' it currently resolves somewhere else, double check the record you added.'
              : ''}
          </p>
          <div className="space-y-1">
            <DnsRecordRow label="Name" value={domain} />
            <DnsRecordRow label="Value" value={data.expected_host} />
          </div>
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
          onClick={() => {
            void refetch()
          }}
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
