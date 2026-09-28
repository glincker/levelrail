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
