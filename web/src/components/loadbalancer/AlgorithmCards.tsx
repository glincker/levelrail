import type { ComponentType } from 'react'
import {
  ArrowsClockwiseIcon,
  ChartPieSliceIcon,
  CookieIcon,
  HashIcon,
  LinkSimpleIcon,
  ScalesIcon,
} from '@phosphor-icons/react/dist/ssr'
import { InfoTip } from '@/components/kit'
import { cn } from '@/lib/utils'
import { ALGORITHM_OPTIONS } from '../../lib/loadBalancer'
import type { LoadBalancerAlgorithm } from '../../queries/appLoadBalancer'

interface Meta {
  icon: ComponentType<{ className?: string; 'aria-hidden'?: boolean }>
  useWhen: string
  tradeoff: string
  lines: [number, number, number]
}

const META: Record<LoadBalancerAlgorithm, Meta> = {
  round_robin: {
    icon: ArrowsClockwiseIcon,
    useWhen: 'Requests cost about the same.',
    tradeoff: 'Simple and fair, but ignores how busy a replica is.',
    lines: [2, 2, 2],
  },
  least_conn: {
    icon: ScalesIcon,
    useWhen: 'Requests vary in duration.',
    tradeoff: 'Evens out load, slightly more bookkeeping per request.',
    lines: [1, 3, 1],
  },
  ip_hash: {
    icon: HashIcon,
    useWhen: 'You need clients to stay put without cookies.',
    tradeoff: 'Clients behind one proxy or CDN all land on one replica.',
    lines: [0, 3, 0],
  },
  uri_hash: {
    icon: LinkSimpleIcon,
    useWhen: 'Replicas keep per-path caches.',
    tradeoff: 'Hot paths can overload one replica.',
    lines: [3, 0, 1],
  },
  cookie: {
    icon: CookieIcon,
    useWhen: 'Sessions live in replica memory.',
    tradeoff: 'Pins a browser to one replica, so uneven load is possible.',
    lines: [0, 3, 0],
  },
  weighted: {
    icon: ChartPieSliceIcon,
    useWhen: 'Canaries or replicas of different sizes.',
    tradeoff: 'You maintain the weights by hand.',
    lines: [4, 1, 1],
  },
}

function MiniDiagram({ lines }: { lines: [number, number, number] }) {
  const ys = [6, 18, 30]
  return (
    <svg viewBox="0 0 64 36" className="h-9 w-16 shrink-0" aria-hidden="true">
      <circle cx="6" cy="18" r="3" className="fill-muted-foreground" />
      {lines.map((w, i) => (
        <g key={i}>
          <path
            d={`M 9 18 C 30 18, 34 ${ys[i]}, 52 ${ys[i]}`}
            fill="none"
            strokeLinecap="round"
            strokeWidth={w || 1}
            strokeDasharray={w ? undefined : '1 3'}
            className={w ? 'stroke-primary' : 'stroke-border'}
          />
          <circle
            cx="56"
            cy={ys[i]}
            r="3"
            className={w ? 'fill-primary' : 'fill-border'}
          />
        </g>
      ))}
    </svg>
  )
}

export function AlgorithmCards({
  value,
  onChange,
}: {
  value: LoadBalancerAlgorithm
  onChange: (algorithm: LoadBalancerAlgorithm) => void
}) {
  return (
    <div
      role="radiogroup"
      aria-label="Balancing algorithm"
      className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3"
    >
      {ALGORITHM_OPTIONS.map((o) => {
        const meta = META[o.value]
        const Icon = meta.icon
        const selected = value === o.value
        return (
          <div key={o.value} className="relative">
            <button
              type="button"
              role="radio"
              aria-checked={selected}
              onClick={() => onChange(o.value)}
              className={cn(
                'flex w-full items-start gap-3 rounded-xl border p-3 text-left outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring/60',
                selected ? 'border-primary bg-primary/5' : 'hover:bg-muted/50',
              )}
            >
              <MiniDiagram lines={meta.lines} />
              <span className="min-w-0 pr-5">
                <span className="flex items-center gap-1.5 text-sm font-medium">
                  <Icon className="size-4" aria-hidden />
                  {o.label}
                </span>
                <span className="block text-xs text-muted-foreground">
                  {meta.useWhen}
                </span>
              </span>
            </button>
            <span className="absolute top-2 right-2">
              <InfoTip label={`About ${o.label}`}>
                {o.description} {meta.tradeoff}
              </InfoTip>
            </span>
          </div>
        )
      })}
    </div>
  )
}
