import {
  ArrowsClockwiseIcon,
  CheckCircleIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import type { VariantProps } from 'class-variance-authority'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useDomainCheck, type DomainCheckStatus } from '../queries/domainCheck'

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
        <p className="mt-2 text-xs text-muted-foreground">
          Add this DNS record at your registrar:{' '}
          <span className="font-medium text-foreground">A record</span>{' '}
          pointing <code className="font-mono">{domain}</code> to{' '}
          <code className="font-mono">{data.expected_host}</code>.
          {data.status === 'resolves_elsewhere'
            ? ' It currently resolves somewhere else, double check the record you added.'
            : ''}
        </p>
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
