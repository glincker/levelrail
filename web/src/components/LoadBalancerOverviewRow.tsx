import { Link } from '@tanstack/react-router'
import {
  CaretRightIcon,
  CheckCircleIcon,
  MinusCircleIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { StatusBadge } from '@/components/ui/status-badge'
import {
  LB_STATE_LABEL,
  type LoadBalancerOverviewState,
  type LoadBalancerSummary,
} from '../queries/loadBalancers'

export const LB_LIST_GRID =
  'grid grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)_1rem] items-center gap-3'

const STATE_META = {
  balancing: {
    variant: 'success',
    label: LB_STATE_LABEL.balancing,
    icon: CheckCircleIcon,
  },
  degraded: {
    variant: 'warning',
    label: LB_STATE_LABEL.degraded,
    icon: WarningIcon,
  },
  none: { variant: 'muted', label: LB_STATE_LABEL.none, icon: MinusCircleIcon },
} as const satisfies Record<
  LoadBalancerOverviewState,
  {
    variant: 'success' | 'warning' | 'muted'
    label: string
    icon: typeof CheckCircleIcon
  }
>

function lastCheckLabel(iso: string | undefined): string {
  if (!iso) return 'Not yet'
  const secs = Math.max(0, Math.round((Date.now() - Date.parse(iso)) / 1000))
  if (secs < 60) return `${secs}s ago`
  if (secs < 3600) return `${Math.round(secs / 60)}m ago`
  return `${Math.round(secs / 3600)}h ago`
}

export function LoadBalancerOverviewHeader() {
  return (
    <div
      className={`${LB_LIST_GRID} sticky top-0 z-10 border-b border-border bg-card px-4 py-2 text-xs font-medium tracking-wide text-muted-foreground uppercase`}
    >
      <span>App</span>
      <span>Algorithm</span>
      <span>State</span>
      <span>Upstreams</span>
      <span>Last check</span>
      <span aria-hidden="true" />
    </div>
  )
}

export function LoadBalancerOverviewRow({
  item,
}: {
  item: LoadBalancerSummary
}) {
  const meta = STATE_META[item.state]
  return (
    <div
      className={`${LB_LIST_GRID} relative h-full w-full border-b border-border px-4 py-3 text-sm transition-colors hover:bg-muted/60`}
    >
      <Link
        to="/apps/$name/loadbalancer"
        params={{ name: item.app }}
        className="truncate font-medium text-foreground after:absolute after:inset-0"
      >
        {item.app}
      </Link>
      <span className="truncate font-mono text-xs text-muted-foreground">
        {item.algorithm}
      </span>
      <span>
        <StatusBadge
          variant={meta.variant}
          label={meta.label}
          icon={meta.icon}
        />
      </span>
      <span className="text-muted-foreground">
        {item.upstreams_total === 0
          ? 'None observed'
          : `${item.upstreams_healthy} of ${item.upstreams_total} healthy`}
      </span>
      <span className="text-muted-foreground">
        {lastCheckLabel(item.last_check)}
      </span>
      <CaretRightIcon
        className="size-4 text-muted-foreground"
        aria-hidden="true"
      />
    </div>
  )
}
