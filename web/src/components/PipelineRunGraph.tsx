import { useMemo, useRef, type KeyboardEvent } from 'react'
import type { PipelineJob, PipelineStatus } from '../types/pipelines'
import {
  baseJobName,
  formatDuration,
  STATUS_LABEL,
} from '../lib/pipelineStatus'
import {
  layoutGraph,
  neighborKey,
  NODE_WIDTH,
  type Direction,
  type GraphMember,
  type GraphNode,
} from '../lib/pipelineGraph'
import { cn } from '@/lib/utils'
import { PipelineStatusIcon } from './PipelineStatusBadge'

const ICON_TONE: Record<PipelineStatus, string> = {
  succeeded: 'text-green-600 dark:text-green-500',
  failed: 'text-destructive',
  running: 'text-primary',
  waiting_approval: 'text-amber-600 dark:text-amber-500',
  queued: 'text-muted-foreground',
  pending: 'text-muted-foreground',
  cancelled: 'text-muted-foreground',
  skipped: 'text-muted-foreground',
}

const NODE_STROKE: Record<PipelineStatus, string> = {
  succeeded: 'stroke-green-300 dark:stroke-green-800',
  failed: 'stroke-destructive/60',
  running: 'stroke-primary/70 animate-pulse motion-reduce:animate-none',
  waiting_approval: 'stroke-amber-400 dark:stroke-amber-700',
  queued: 'stroke-border',
  pending: 'stroke-border',
  cancelled: 'stroke-border',
  skipped: 'stroke-border',
}

const ARROWS: Record<string, Direction> = {
  ArrowLeft: 'left',
  ArrowRight: 'right',
  ArrowUp: 'up',
  ArrowDown: 'down',
}

function clip(text: string, max: number): string {
  return text.length > max ? `${text.slice(0, max - 1)}…` : text
}

function MemberButton({
  member,
  width,
  selected,
  clipAt,
  onSelect,
  onKeyDown,
}: {
  member: GraphMember
  width: number
  selected: boolean
  clipAt: number
  onSelect: (key: string) => void
  onKeyDown: (e: KeyboardEvent<SVGGElement>, key: string) => void
}) {
  const duration = formatDuration(member.job.started_at, member.job.finished_at)
  const cy = member.height / 2
  return (
    <g
      transform={`translate(0 ${member.y})`}
      role="button"
      tabIndex={0}
      aria-pressed={selected}
      aria-label={`${member.job.key}, ${STATUS_LABEL[member.status]}${duration ? `, ${duration}` : ''}`}
      data-job-key={member.key}
      className="group cursor-pointer outline-none"
      onClick={() => onSelect(member.key)}
      onKeyDown={(e) => onKeyDown(e, member.key)}
    >
      <title>{member.job.key}</title>
      <rect
        width={width}
        height={member.height}
        rx={6}
        className="fill-transparent group-hover:fill-muted group-focus-visible:stroke-ring group-focus-visible:stroke-2"
      />
      {selected ? (
        <rect
          width={width}
          height={member.height}
          rx={6}
          className="fill-primary/10 stroke-primary stroke-2"
        />
      ) : null}
      <g transform={`translate(10 ${cy - 8})`}>
        <PipelineStatusIcon
          status={member.status}
          spin={false}
          className={cn('size-4', ICON_TONE[member.status])}
        />
      </g>
      <text
        x={32}
        y={cy}
        dominantBaseline="central"
        className="fill-foreground text-xs font-medium"
      >
        {clip(member.label, clipAt)}
      </text>
      <text
        x={width - 10}
        y={cy}
        textAnchor="end"
        dominantBaseline="central"
        className="fill-muted-foreground text-[11px] tabular-nums"
      >
        {duration}
      </text>
    </g>
  )
}

function NodeView({
  node,
  selected,
  onSelect,
  onKeyDown,
}: {
  node: GraphNode
  selected: string
  onSelect: (key: string) => void
  onKeyDown: (e: KeyboardEvent<SVGGElement>, key: string) => void
}) {
  const inner = node.members.map((m) => (
    <MemberButton
      key={m.key}
      member={m}
      width={node.width}
      selected={selected === m.key}
      clipAt={node.grouped ? 26 : 19}
      onSelect={onSelect}
      onKeyDown={onKeyDown}
    />
  ))
  return (
    <g transform={`translate(${node.x} ${node.y})`}>
      <rect
        width={node.width}
        height={node.height}
        rx={8}
        className={cn('fill-card stroke-1', NODE_STROKE[node.status])}
      />
      {node.grouped ? (
        <g>
          <g transform="translate(10 6)">
            <PipelineStatusIcon
              status={node.status}
              spin={false}
              className={cn('size-4', ICON_TONE[node.status])}
            />
          </g>
          <text
            x={32}
            y={14}
            dominantBaseline="central"
            className="fill-foreground text-xs font-semibold"
          >
            {clip(node.label, 20)}
          </text>
          <text
            x={node.width - 10}
            y={14}
            textAnchor="end"
            dominantBaseline="central"
            className="fill-muted-foreground text-[11px]"
          >
            {node.members.length} runs
          </text>
        </g>
      ) : null}
      {inner}
    </g>
  )
}

// The run as a dependency graph: columns by depth, curved edges from each
// needed job to the job that needs it, matrix jobs stacked in one node.
export function PipelineRunGraph({
  jobs,
  selected,
  onSelect,
}: {
  jobs: PipelineJob[]
  selected: string
  onSelect: (key: string) => void
}) {
  const layout = useMemo(() => layoutGraph(jobs), [jobs])
  const svgRef = useRef<SVGSVGElement>(null)
  const focusGroup = baseJobName(selected)

  const onKeyDown = (e: KeyboardEvent<SVGGElement>, key: string) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      onSelect(key)
      return
    }
    const dir = ARROWS[e.key]
    if (!dir) {
      return
    }
    e.preventDefault()
    const next = neighborKey(layout, key, dir)
    if (!next) {
      return
    }
    const el = Array.from(
      svgRef.current?.querySelectorAll<SVGGElement>('[data-job-key]') ?? [],
    ).find((n) => n.getAttribute('data-job-key') === next)
    el?.focus()
  }

  const linked = (e: { from: string; to: string }) =>
    e.from === focusGroup || e.to === focusGroup

  return (
    <div className="max-h-[36rem] overflow-auto rounded-lg border border-border bg-muted/20 p-2">
      <svg
        ref={svgRef}
        width={Math.max(layout.width, NODE_WIDTH)}
        height={layout.height}
        role="group"
        aria-label="Pipeline jobs and their dependencies"
        className="block"
      >
        <g fill="none" aria-hidden="true">
          {layout.edges
            .filter((e) => !linked(e))
            .map((e) => (
              <path
                key={`${e.from}>${e.to}`}
                d={e.path}
                data-edge=""
                className="stroke-border stroke-[1.5]"
              />
            ))}
          {layout.edges.filter(linked).map((e) => (
            <path
              key={`${e.from}>${e.to}`}
              d={e.path}
              data-edge=""
              className="stroke-primary/70 stroke-2"
            />
          ))}
        </g>
        {layout.nodes.map((n) => (
          <NodeView
            key={n.id}
            node={n}
            selected={selected}
            onSelect={onSelect}
            onKeyDown={onKeyDown}
          />
        ))}
      </svg>
    </div>
  )
}
