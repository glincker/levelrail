import { selectableCardClass } from '@/components/ui/selectable-card'
import type { ComponentType } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ChartColumnGlassIcon,
  CookieGlassIcon,
  FingerprintPatternGlassIcon,
  HashGlassIcon,
  LinkGlassIcon,
  RefreshCwGlassIcon,
  ScaleGlassIcon,
} from '@thesvg/ui-react'
import { InfoTip } from '@/components/kit'
import { cn } from '@/lib/utils'
import { ALGORITHM_OPTIONS } from '../../lib/loadBalancer'
import type { LoadBalancerAlgorithm } from '../../queries/appLoadBalancer'

type AlgoKey =
  | 'roundRobin'
  | 'leastConn'
  | 'ipHash'
  | 'clientIpHash'
  | 'uriHash'
  | 'cookie'
  | 'weighted'

interface Meta {
  icon: ComponentType<{ className?: string; 'aria-hidden'?: boolean }>
  useWhen: `lb.algo.${AlgoKey}Use`
  tradeoff: `lb.algo.${AlgoKey}Tradeoff`
  lines: [number, number, number]
}

const META: Record<LoadBalancerAlgorithm, Meta> = {
  round_robin: {
    icon: RefreshCwGlassIcon,
    useWhen: 'lb.algo.roundRobinUse',
    tradeoff: 'lb.algo.roundRobinTradeoff',
    lines: [2, 2, 2],
  },
  least_conn: {
    icon: ScaleGlassIcon,
    useWhen: 'lb.algo.leastConnUse',
    tradeoff: 'lb.algo.leastConnTradeoff',
    lines: [1, 3, 1],
  },
  ip_hash: {
    icon: HashGlassIcon,
    useWhen: 'lb.algo.ipHashUse',
    tradeoff: 'lb.algo.ipHashTradeoff',
    lines: [0, 3, 0],
  },
  client_ip_hash: {
    icon: FingerprintPatternGlassIcon,
    useWhen: 'lb.algo.clientIpHashUse',
    tradeoff: 'lb.algo.clientIpHashTradeoff',
    lines: [0, 3, 0],
  },
  uri_hash: {
    icon: LinkGlassIcon,
    useWhen: 'lb.algo.uriHashUse',
    tradeoff: 'lb.algo.uriHashTradeoff',
    lines: [3, 0, 1],
  },
  cookie: {
    icon: CookieGlassIcon,
    useWhen: 'lb.algo.cookieUse',
    tradeoff: 'lb.algo.cookieTradeoff',
    lines: [0, 3, 0],
  },
  weighted: {
    icon: ChartColumnGlassIcon,
    useWhen: 'lb.algo.weightedUse',
    tradeoff: 'lb.algo.weightedTradeoff',
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
  const { t } = useTranslation('statePlaceholders')
  return (
    <div
      role="radiogroup"
      aria-label={t('lb.algo.label')}
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
                'flex w-full items-start gap-3 rounded-xl p-3 text-left',
                selectableCardClass(selected),
              )}
            >
              <MiniDiagram lines={meta.lines} />
              <span className="min-w-0 pr-5">
                <span className="flex items-center gap-1.5 text-sm font-medium">
                  <Icon className="size-5" aria-hidden />
                  {o.label}
                </span>
                <span className="block text-xs text-muted-foreground">
                  {t(meta.useWhen)}
                </span>
              </span>
            </button>
            <span className="absolute top-2 right-2">
              <InfoTip label={t('lb.algo.about', { name: o.label })}>
                {o.description} {t(meta.tradeoff)}
              </InfoTip>
            </span>
          </div>
        )
      })}
    </div>
  )
}
