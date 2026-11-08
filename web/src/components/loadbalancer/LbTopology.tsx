import { useTranslation } from 'react-i18next'
import {
  PauseGlassIcon,
  PlayGlassIcon,
  ServerGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { EmptyState } from '@/components/ui/empty-state'
import { InfoTip, useReducedMotion } from '@/components/kit'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import type { LoadBalancerAlgorithm } from '../../queries/appLoadBalancer'
import type { LiveUpstream } from '../../queries/loadBalancerLive'
import { formatShare, rollupPool, upstreamShares, upstreamView } from './rollup'
import {
  CLIENT,
  MAX_NODES,
  NODE_H,
  PROXY,
  UP,
  VIEW_W,
  flowSeconds,
  nodeY,
  trafficShares,
  viewHeight,
} from './topologyGeometry'
import { SVG_TONE } from './svgTone'
import { NodeGlyph } from './TopologyNode'

interface Props {
  upstreams: LiveUpstream[]
  algorithm: LoadBalancerAlgorithm
  paused: boolean
  onPausedChange: (paused: boolean) => void
  onSelect: (id: string) => void
}

export function LbTopology({
  upstreams,
  algorithm,
  paused,
  onPausedChange,
  onSelect,
}: Props) {
  const { t } = useTranslation('statePlaceholders')
  const reduced = useReducedMotion()
  const animate = !paused && !reduced
  const shown = upstreams.slice(0, MAX_NODES)
  const hidden = upstreams.length - shown.length
  const height = viewHeight(shown.length, hidden > 0)
  const midY = height / 2
  const shares = trafficShares(shown)
  const pct = upstreamShares(upstreams, algorithm)
  const rollup = rollupPool(upstreams)

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="flex items-center gap-1 text-sm font-medium">
          {t('lb.topology.traffic')}
          <InfoTip label={t('lb.topology.howToRead')} side="right">
            {t('lb.topology.howToReadBody')}
          </InfoTip>
        </span>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          aria-pressed={!animate}
          disabled={reduced}
          onClick={() => onPausedChange(!paused)}
        >
          {animate ? (
            <PauseGlassIcon size="1.4em" data-icon="inline-start" />
          ) : (
            <PlayGlassIcon size="1.4em" data-icon="inline-start" />
          )}
          {reduced
            ? t('lb.topology.motionOff')
            : animate
              ? t('lb.topology.pause')
              : t('lb.topology.play')}
        </Button>
      </div>

      <svg
        role="group"
        aria-label={t('lb.topology.diagram', {
          total: upstreams.length,
          healthy: rollup.healthy,
        })}
        viewBox={`0 0 ${VIEW_W} ${height}`}
        className="h-auto w-full max-w-[560px]"
      >
        <line
          x1={CLIENT.x + CLIENT.w}
          y1={midY}
          x2={PROXY.x}
          y2={midY}
          className="stroke-muted-foreground"
          strokeWidth={2}
          strokeDasharray="6 4"
        >
          {animate ? (
            <animate
              attributeName="stroke-dashoffset"
              from="0"
              to="-20"
              dur="1.2s"
              repeatCount="indefinite"
            />
          ) : null}
        </line>

        {shown.map((u, i) => {
          const view = upstreamView(u)
          const tone = SVG_TONE[view.tone]
          const cy = nodeY(i) + NODE_H / 2
          const x1 = PROXY.x + PROXY.w
          const share = shares[i] ?? 0
          const flowing = view.glyph === 'ok'
          const width = flowing ? 1.5 + share * 3 : 1.5
          const dash = flowing
            ? `${4 + share * 6} ${Math.round(14 - share * 9)}`
            : view.glyph === 'warn'
              ? '6 5'
              : '1.5 6'
          const path = `M ${x1} ${midY} C ${x1 + 26} ${midY}, ${UP.x - 26} ${cy}, ${UP.x} ${cy}`
          return (
            <g key={u.id}>
              <path
                d={path}
                fill="none"
                className={tone.stroke}
                strokeWidth={width}
                strokeDasharray={dash}
                strokeLinecap="round"
                data-edge={view.glyph}
              >
                {animate && flowing ? (
                  <animate
                    attributeName="stroke-dashoffset"
                    from="0"
                    to="-30"
                    dur={`${flowSeconds(u.latency_ms)}s`}
                    repeatCount="indefinite"
                  />
                ) : null}
              </path>
              <g
                role="button"
                tabIndex={0}
                aria-label={t('lb.topology.node', {
                  name:
                    u.dial || t('lb.topology.replica', { replica: u.replica }),
                  label: view.label,
                  reason: view.reason,
                })}
                data-node={u.id}
                className="group cursor-pointer outline-none"
                onClick={() => onSelect(u.id)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault()
                    onSelect(u.id)
                  }
                }}
              >
                <rect
                  x={UP.x}
                  y={nodeY(i)}
                  width={UP.w}
                  height={NODE_H}
                  rx={10}
                  className={cn(
                    tone.fill,
                    tone.border,
                    'stroke-1 transition-[stroke-width] group-hover:stroke-2 group-focus-visible:stroke-[3] group-focus-visible:stroke-ring',
                  )}
                />
                <NodeGlyph
                  glyph={view.glyph}
                  cx={UP.x + 20}
                  cy={cy}
                  tone={view.tone}
                />
                <text
                  x={UP.x + 38}
                  y={cy - 3}
                  className="fill-foreground text-xs font-medium"
                >
                  {t('lb.topology.replica', { replica: u.replica })}
                </text>
                <text
                  x={UP.x + 38}
                  y={cy + 12}
                  className="fill-muted-foreground text-xs"
                >
                  {`${view.label}${pct.get(u.id) ? `, ${formatShare(pct.get(u.id) ?? 0, algorithm)}` : ''}`}
                </text>
              </g>
            </g>
          )
        })}

        {hidden > 0 ? (
          <text
            x={UP.x}
            y={height - 8}
            className="fill-muted-foreground text-xs"
          >
            {t('lb.topology.more', { count: hidden })}
          </text>
        ) : null}

        <g>
          <rect
            x={CLIENT.x}
            y={midY - 22}
            width={CLIENT.w}
            height={44}
            rx={10}
            className="fill-muted stroke-border"
          />
          <text
            x={CLIENT.x + CLIENT.w / 2}
            y={midY + 4}
            textAnchor="middle"
            className="fill-foreground text-xs font-medium"
          >
            {t('lb.topology.clients')}
          </text>
        </g>
        <g>
          <rect
            x={PROXY.x}
            y={midY - 26}
            width={PROXY.w}
            height={52}
            rx={12}
            className="fill-tone-accent-soft stroke-tone-accent-border"
          />
          <text
            x={PROXY.x + PROXY.w / 2}
            y={midY - 2}
            textAnchor="middle"
            className="fill-foreground text-xs font-medium"
          >
            {t('lb.topology.proxy')}
          </text>
          <text
            x={PROXY.x + PROXY.w / 2}
            y={midY + 13}
            textAnchor="middle"
            className="fill-muted-foreground text-xs"
          >
            {algorithm.replace('_', ' ')}
          </text>
        </g>
      </svg>

      {rollup.total === 0 ? (
        <EmptyState
          kind="no-results"
          icon={<ServerGlassIcon className="size-5" aria-hidden="true" />}
          title={t('lb.topology.noUpstreams')}
          description={t('lb.topology.noUpstreamsHint')}
        />
      ) : null}
      {rollup.allDown ? (
        <Alert variant="danger" role="status">
          <TriangleAlertGlassIcon aria-hidden="true" />
          <AlertDescription>{t('lb.topology.allDown')}</AlertDescription>
        </Alert>
      ) : null}

      <table className="sr-only">
        <caption>{t('lb.topology.caption')}</caption>
        <thead>
          <tr>
            <th scope="col">{t('lb.topology.upstream')}</th>
            <th scope="col">{t('lb.topology.state')}</th>
            <th scope="col">{t('lb.topology.reason')}</th>
            <th scope="col">{t('lb.topology.share')}</th>
            <th scope="col">{t('lb.topology.connections')}</th>
          </tr>
        </thead>
        <tbody>
          {upstreams.map((u) => {
            const view = upstreamView(u)
            return (
              <tr key={u.id}>
                <th scope="row">
                  {u.dial || t('lb.topology.replica', { replica: u.replica })}
                </th>
                <td>{view.label}</td>
                <td>{view.reason}</td>
                <td>{formatShare(pct.get(u.id) ?? 0, algorithm)}</td>
                <td>{u.active_connections}</td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
