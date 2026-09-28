import type { Tone } from '@/components/kit'

export const SVG_TONE: Record<
  Tone,
  { fill: string; border: string; stroke: string; text: string }
> = {
  neutral: {
    fill: 'fill-tone-neutral-soft',
    border: 'stroke-tone-neutral-border',
    stroke: 'stroke-tone-neutral-solid',
    text: 'fill-tone-neutral',
  },
  success: {
    fill: 'fill-tone-success-soft',
    border: 'stroke-tone-success-border',
    stroke: 'stroke-tone-success-solid',
    text: 'fill-tone-success',
  },
  warning: {
    fill: 'fill-tone-warning-soft',
    border: 'stroke-tone-warning-border',
    stroke: 'stroke-tone-warning-solid',
    text: 'fill-tone-warning',
  },
  danger: {
    fill: 'fill-tone-danger-soft',
    border: 'stroke-tone-danger-border',
    stroke: 'stroke-tone-danger-solid',
    text: 'fill-tone-danger',
  },
  info: {
    fill: 'fill-tone-info-soft',
    border: 'stroke-tone-info-border',
    stroke: 'stroke-tone-info-solid',
    text: 'fill-tone-info',
  },
  accent: {
    fill: 'fill-tone-accent-soft',
    border: 'stroke-tone-accent-border',
    stroke: 'stroke-tone-accent-solid',
    text: 'fill-tone-accent',
  },
}
