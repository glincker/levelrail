import { PauseIcon, PlayIcon } from '@phosphor-icons/react/dist/ssr'
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
      <div className="flex items-center justify-between gap-2">
        <span className="flex items-center gap-1 text-sm font-medium">
          Traffic
          <InfoTip label="How to read the diagram" side="right">
            Line density shows each replica&apos;s share of open connections.
            Faster motion means lower latency. Shapes and labels show health:
            check circle is healthy, triangle is draining, square is down.
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
            <PauseIcon data-icon="inline-start" />
          ) : (
            <PlayIcon data-icon="inline-start" />
          )}
          {reduced ? 'Motion off' : animate ? 'Pause' : 'Play'}
        </Button>
      </div>

      <svg
        role="group"
        aria-label={`Traffic from clients through the proxy to ${upstreams.length} upstreams, ${rollup.healthy} healthy`}
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
                aria-label={`${u.dial || `Replica ${u.replica}`}: ${view.label}, ${view.reason}. Open details`}
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
                  className="fill-foreground text-[12px] font-medium"
                >
                  {`Replica ${u.replica}`}
                </text>
                <text
                  x={UP.x + 38}
                  y={cy + 12}
                  className="fill-muted-foreground text-[11px]"
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
            className="fill-muted-foreground text-[11px]"
          >{`+${hidden} more in the table below`}</text>
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
            className="fill-foreground text-[12px] font-medium"
          >
            Clients
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
            className="fill-foreground text-[12px] font-medium"
          >
            Proxy
          </text>
          <text
            x={PROXY.x + PROXY.w / 2}
            y={midY + 13}
            textAnchor="middle"
            className="fill-muted-foreground text-[11px]"
          >
            {algorithm.replace('_', ' ')}
          </text>
        </g>
      </svg>

      {rollup.total === 0 ? (
        <p className="text-sm text-muted-foreground">
          No upstreams yet. They appear as replicas start.
        </p>
      ) : null}
      {rollup.allDown ? (
        <p role="status" className="text-sm text-tone-danger">
          Nothing is passing checks. Requests may fail until a replica recovers.
        </p>
      ) : null}

      <table className="sr-only">
        <caption>Upstream status</caption>
        <thead>
          <tr>
            <th scope="col">Upstream</th>
            <th scope="col">State</th>
            <th scope="col">Reason</th>
            <th scope="col">Share</th>
            <th scope="col">Connections</th>
          </tr>
        </thead>
        <tbody>
          {upstreams.map((u) => {
            const view = upstreamView(u)
            return (
              <tr key={u.id}>
                <th scope="row">{u.dial || `Replica ${u.replica}`}</th>
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
