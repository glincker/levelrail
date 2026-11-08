export type Tone =
  'neutral' | 'success' | 'warning' | 'danger' | 'info' | 'accent'

export interface ToneClasses {
  text: string
  soft: string
  border: string
  solid: string
}

export const TONE: Record<Tone, ToneClasses> = {
  neutral: {
    text: 'text-tone-neutral',
    soft: 'bg-tone-neutral-soft',
    border: 'border-tone-neutral-border',
    solid: 'bg-tone-neutral-solid',
  },
  success: {
    text: 'text-tone-success',
    soft: 'bg-tone-success-soft',
    border: 'border-tone-success-border',
    solid: 'bg-tone-success-solid',
  },
  warning: {
    text: 'text-tone-warning',
    soft: 'bg-tone-warning-soft',
    border: 'border-tone-warning-border',
    solid: 'bg-tone-warning-solid',
  },
  danger: {
    text: 'text-tone-danger',
    soft: 'bg-tone-danger-soft',
    border: 'border-tone-danger-border',
    solid: 'bg-tone-danger-solid',
  },
  info: {
    text: 'text-tone-info',
    soft: 'bg-tone-info-soft',
    border: 'border-tone-info-border',
    solid: 'bg-tone-info-solid',
  },
  accent: {
    text: 'text-tone-accent',
    soft: 'bg-tone-accent-soft',
    border: 'border-tone-accent-border',
    solid: 'bg-tone-accent-solid',
  },
}

export type StatusShape = 'circle' | 'ring' | 'triangle' | 'diamond' | 'dashed'

export interface StatusSpec {
  tone: Tone
  shape: StatusShape
  live: boolean
}

// Shape carries meaning without colour; only in-progress states ping.
export const STATUS = {
  running: { tone: 'success', shape: 'circle', live: false },
  healthy: { tone: 'success', shape: 'circle', live: false },
  ready: { tone: 'success', shape: 'circle', live: false },
  building: { tone: 'info', shape: 'ring', live: true },
  deploying: { tone: 'info', shape: 'ring', live: true },
  waking: { tone: 'info', shape: 'ring', live: true },
  draining: { tone: 'warning', shape: 'triangle', live: true },
  degraded: { tone: 'warning', shape: 'triangle', live: false },
  partial: { tone: 'warning', shape: 'triangle', live: false },
  failed: { tone: 'danger', shape: 'diamond', live: false },
  offline: { tone: 'danger', shape: 'diamond', live: false },
  expired: { tone: 'danger', shape: 'diamond', live: false },
  queued: { tone: 'neutral', shape: 'dashed', live: false },
  scheduled: { tone: 'neutral', shape: 'dashed', live: false },
  pending: { tone: 'neutral', shape: 'dashed', live: false },
  stopped: { tone: 'neutral', shape: 'dashed', live: false },
  paused: { tone: 'neutral', shape: 'dashed', live: false },
  sleeping: { tone: 'neutral', shape: 'dashed', live: false },
} as const satisfies Record<string, StatusSpec>

export type StatusState = keyof typeof STATUS

export type BadgeToneVariant =
  'muted' | 'success' | 'warning' | 'destructive' | 'info' | 'accent'

// One vocabulary: Badge, StatusBadge and StatusPill all resolve a Tone through this table.
export const TONE_BADGE_VARIANT: Record<Tone, BadgeToneVariant> = {
  neutral: 'muted',
  success: 'success',
  warning: 'warning',
  danger: 'destructive',
  info: 'info',
  accent: 'accent',
}
