import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  CircleGlassIcon,
  CircleMinusGlassIcon,
  LoaderCircleGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { useDeployDetailT } from './deployDetail/useDeployDetailT'
import type {
  DeployStepEvent,
  DeployStepStatus,
} from '../hooks/useDeployStepStream'
import { isTraceStep } from '@/lib/deployTrace'
import { cn } from '@/lib/utils'

// The pipeline stages of a traced deploy, in order. A step name not listed
// here still renders, under the last group, so a newer server never hides a step.
const GROUPS: { id: string; steps: string[] }[] = [
  {
    id: 'compose',
    steps: ['compose_parse', 'compose_plan', 'compose_service', 'depends_on'],
  },
  {
    id: 'image',
    steps: [
      'resolve_digest',
      'registry_auth',
      'image_pull',
      'image_pull_layer',
      'image_inspect',
    ],
  },
  {
    id: 'prepare',
    steps: ['config_validate', 'ensure_network', 'ensure_volume'],
  },
  {
    id: 'container',
    steps: ['container_create', 'container_start', 'docker_event'],
  },
  {
    id: 'readiness',
    steps: ['readiness_probe', 'readiness_gate', 'stability_gate'],
  },
  { id: 'cutover', steps: ['cutover', 'drain_old', 'retain_image'] },
  { id: 'summary', steps: ['deploy_summary'] },
]

// Step kinds shown folded under a count, since a deploy can have dozens.
const FOLDED = new Set(['image_pull_layer', 'readiness_probe', 'docker_event'])

function StatusIcon({ status }: { status: DeployStepStatus }) {
  const base = 'size-4 shrink-0'
  switch (status) {
    case 'done':
      return (
        <CircleCheckGlassIcon
          size="1.2em"
          className={cn(base, 'text-success-foreground')}
          aria-hidden="true"
        />
      )
    case 'running':
      return (
        <LoaderCircleGlassIcon
          size="1.2em"
          className={cn(base, 'animate-spin text-muted-foreground')}
          aria-hidden="true"
        />
      )
    case 'failed':
      return (
        <CircleAlertGlassIcon
          size="1.2em"
          className={cn(base, 'text-destructive')}
          aria-hidden="true"
        />
      )
    case 'warning':
      return (
        <TriangleAlertGlassIcon
          size="1.2em"
          className={cn(base, 'text-warning-foreground')}
          aria-hidden="true"
        />
      )
    case 'skipped':
      return (
        <CircleMinusGlassIcon
          size="1.2em"
          className={cn(base, 'text-muted-foreground')}
          aria-hidden="true"
        />
      )
    default:
      return (
        <CircleGlassIcon
          size="1.2em"
          className={cn(base, 'text-muted-foreground/50')}
          aria-hidden="true"
        />
      )
  }
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms} ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)} s`
  return `${Math.floor(ms / 60_000)} m ${Math.round((ms % 60_000) / 1000)} s`
}

function pullPercent(ev: DeployStepEvent): number | null {
  if (ev.step !== 'image_pull' || ev.status !== 'running') return null
  const m = /^(\d{1,3})%/.exec(ev.detail ?? '')
  return m ? Math.min(100, Number(m[1])) : null
}

function TraceRow({ ev }: { ev: DeployStepEvent }) {
  const { t } = useDeployDetailT()
  const id = ev.key ?? ev.step
  const suffix = id.startsWith(`${ev.step}.`)
    ? id.slice(ev.step.length + 1)
    : ''
  const percent = pullPercent(ev)
  return (
    <li className="flex items-start gap-2 text-sm">
      <span className="mt-0.5">
        <StatusIcon status={ev.status} />
      </span>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-baseline gap-x-2">
          <span
            className={cn(
              ev.status === 'failed' && 'text-destructive',
              ev.status === 'skipped' && 'text-muted-foreground',
            )}
          >
            {t(`trace.step.${ev.step}`, { defaultValue: ev.step })}
            {suffix ? ` ${suffix}` : ''}
            {ev.service ? ` (${ev.service})` : ''}
          </span>
          {ev.duration_ms ? (
            <span className="text-xs text-muted-foreground">
              {formatDuration(ev.duration_ms)}
            </span>
          ) : null}
          {ev.reason ? (
            <span className="rounded-sm bg-muted px-1.5 text-xs text-muted-foreground">
              {ev.reason}
            </span>
          ) : null}
          {ev.attempts && ev.attempts > 1 ? (
            <span className="text-xs text-muted-foreground">
              {t('trace.attempts', { count: ev.attempts })}
            </span>
          ) : null}
        </div>
        {percent !== null ? (
          <progress
            className="mt-1 h-1.5 w-full max-w-xs"
            value={percent}
            max={100}
            aria-label={t('trace.pullProgress')}
          />
        ) : null}
        {ev.detail ? (
          <p className="break-words text-xs text-muted-foreground">
            {ev.detail}
          </p>
        ) : null}
      </div>
    </li>
  )
}

function groupOf(step: string): string {
  return GROUPS.find((g) => g.steps.includes(step))?.id ?? 'summary'
}

function GroupRows({ rows }: { rows: DeployStepEvent[] }) {
  const { t } = useDeployDetailT()
  const plain = rows.filter((r) => !FOLDED.has(r.step))
  const folded = Array.from(
    new Set(rows.filter((r) => FOLDED.has(r.step)).map((r) => r.step)),
  )
  return (
    <>
      {plain.map((ev) => (
        <TraceRow key={ev.key ?? ev.step} ev={ev} />
      ))}
      {folded.map((step) => {
        const items = rows.filter((r) => r.step === step)
        return (
          <li key={step}>
            <details>
              <summary className="cursor-pointer text-sm text-muted-foreground">
                {t(`trace.folded.${step}`, { count: items.length })}
              </summary>
              <ol className="mt-1 flex flex-col gap-1.5 pl-6">
                {items.map((ev) => (
                  <TraceRow key={ev.key ?? ev.step} ev={ev} />
                ))}
              </ol>
            </details>
          </li>
        )
      })}
    </>
  )
}

// DeployTraceList renders the pipeline trace every deploy carries (build or
// not), grouped by stage. It reads the same step events as DeployStepFeed.
export function DeployTraceList({ steps }: { steps: DeployStepEvent[] }) {
  const { t } = useDeployDetailT()
  const trace = steps.filter(isTraceStep)
  if (trace.length === 0) return null
  return (
    <div className="flex flex-col gap-3">
      {GROUPS.map((g) => {
        const rows = trace.filter((s) => groupOf(s.step) === g.id)
        if (rows.length === 0) return null
        return (
          <section key={g.id} aria-label={t(`trace.group.${g.id}`)}>
            <h4 className="mb-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">
              {t(`trace.group.${g.id}`)}
            </h4>
            <ol className="flex flex-col gap-1.5">
              <GroupRows rows={rows} />
            </ol>
          </section>
        )
      })}
    </div>
  )
}
