import type { Tone } from '../components/kit/tone'
import type {
  DeployEvent,
  DeployEventImpact,
  DeployEventMessage,
  DeployEventPhase,
  DeployEventStatus,
  DeployEventTrigger,
  DeployVerdict,
} from '../types/deployEvents'

export interface DeployEventState {
  deployId: string
  app: string
  verdict: DeployVerdict
  live: boolean
  frozen: boolean
  ended: boolean
  trigger: DeployEventTrigger | null
  events: DeployEvent[]
  phases: DeployEventPhase[]
  impact: DeployEventImpact | null
  logsDropped: number
}

export const EMPTY_EVENT_STATE: DeployEventState = {
  deployId: '',
  app: '',
  verdict: 'running',
  live: true,
  frozen: false,
  ended: false,
  trigger: null,
  events: [],
  phases: [],
  impact: null,
  logsDropped: 0,
}

function byTime(events: DeployEvent[]): DeployEvent[] {
  return events
    .map((e, i) => ({ e, i, t: Date.parse(e.at) }))
    .sort((x, y) => x.t - y.t || x.i - y.i)
    .map((x) => x.e)
}

function upsert(events: DeployEvent[], next: DeployEvent): DeployEvent[] {
  const i = events.findIndex((e) => e.id === next.id)
  if (i === -1) return byTime([...events, next])
  const copy = [...events]
  copy[i] = next
  return byTime(copy)
}

export function applyMessage(
  state: DeployEventState,
  msg: DeployEventMessage,
): DeployEventState {
  switch (msg.type) {
    case 'snapshot': {
      const t = msg.timeline
      return {
        deployId: t.deploy_id,
        app: t.app,
        verdict: t.verdict,
        live: t.live,
        frozen: t.frozen || msg.frozen === true,
        ended: false,
        trigger: t.trigger,
        events: byTime(t.events),
        phases: t.phases,
        impact: t.impact ?? null,
        logsDropped: t.logs_dropped ?? 0,
      }
    }
    case 'event':
      return {
        ...state,
        live: msg.live,
        events: upsert(state.events, msg.event),
        impact: msg.event.impact ?? state.impact,
      }
    case 'state':
      return {
        ...state,
        verdict: msg.verdict,
        live: msg.live,
        phases: msg.phases ?? state.phases,
      }
    case 'end':
      return {
        ...state,
        ended: true,
        live: false,
        frozen: state.frozen || msg.frozen === true,
        verdict: msg.verdict ?? state.verdict,
      }
  }
}

export function parseEventMessage(raw: string): DeployEventMessage | null {
  try {
    const parsed = JSON.parse(raw) as { type?: unknown }
    switch (parsed.type) {
      case 'snapshot':
      case 'event':
      case 'state':
      case 'end':
        return parsed as DeployEventMessage
    }
  } catch {
    return null
  }
  return null
}

export type EventRow =
  | { kind: 'event'; event: DeployEvent }
  | { kind: 'logs'; id: string; lines: DeployEvent[] }

/** Folds consecutive build log lines into one collapsible group. */
export function groupRows(events: DeployEvent[]): EventRow[] {
  const rows: EventRow[] = []
  for (const event of events) {
    if (event.kind !== 'log') {
      rows.push({ kind: 'event', event })
      continue
    }
    const last = rows[rows.length - 1]
    if (last?.kind === 'logs') {
      last.lines.push(event)
    } else {
      rows.push({ kind: 'logs', id: event.id, lines: [event] })
    }
  }
  return rows
}

export function statusTone(status: DeployEventStatus): Tone {
  switch (status) {
    case 'ok':
      return 'success'
    case 'failed':
      return 'danger'
    case 'warning':
      return 'warning'
    case 'running':
      return 'info'
    case 'info':
      return 'neutral'
  }
}

export function verdictTone(verdict: DeployVerdict): Tone {
  switch (verdict) {
    case 'succeeded':
      return 'success'
    case 'failed':
      return 'danger'
    case 'rolled_back':
      return 'warning'
    case 'running':
    case 'watching':
      return 'info'
    case 'canceled':
    case 'queued':
      return 'neutral'
  }
}

export function formatMs(ms: number): string {
  if (ms < 1000) return `${String(Math.round(ms))}ms`
  const s = ms / 1000
  if (s < 60) return `${s.toFixed(s < 10 ? 1 : 0)}s`
  const m = Math.floor(s / 60)
  const rest = Math.round(s % 60)
  return rest === 0 ? `${String(m)}m` : `${String(m)}m ${String(rest)}s`
}

export function formatPercent(ratio: number): string {
  return `${(ratio * 100).toFixed(ratio < 0.1 ? 1 : 0)}%`
}

const WIDTH_STEPS = [
  'w-0',
  'w-[5%]',
  'w-[10%]',
  'w-[15%]',
  'w-[20%]',
  'w-[25%]',
  'w-[30%]',
  'w-[35%]',
  'w-[40%]',
  'w-[45%]',
  'w-[50%]',
  'w-[55%]',
  'w-[60%]',
  'w-[65%]',
  'w-[70%]',
  'w-[75%]',
  'w-[80%]',
  'w-[85%]',
  'w-[90%]',
  'w-[95%]',
  'w-full',
] as const

/** A literal Tailwind width class for value/max, in 5 percent steps. */
export function barWidthClass(value: number, max: number): string {
  if (max <= 0 || value <= 0) return WIDTH_STEPS[0]
  const step = Math.min(
    WIDTH_STEPS.length - 1,
    Math.max(1, Math.round((value / max) * (WIDTH_STEPS.length - 1))),
  )
  return WIDTH_STEPS[step] ?? WIDTH_STEPS[0]
}

export function codeKey(code: string): string {
  return code.replace(/\./g, '_')
}
