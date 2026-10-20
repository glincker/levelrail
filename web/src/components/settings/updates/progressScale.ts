// Quantised to 5% so the bar fill is a static Tailwind transform class
// (no inline style, no width animation).
const SCALES = [
  'scale-x-[0]',
  'scale-x-[0.05]',
  'scale-x-[0.1]',
  'scale-x-[0.15]',
  'scale-x-[0.2]',
  'scale-x-[0.25]',
  'scale-x-[0.3]',
  'scale-x-[0.35]',
  'scale-x-[0.4]',
  'scale-x-[0.45]',
  'scale-x-[0.5]',
  'scale-x-[0.55]',
  'scale-x-[0.6]',
  'scale-x-[0.65]',
  'scale-x-[0.7]',
  'scale-x-[0.75]',
  'scale-x-[0.8]',
  'scale-x-[0.85]',
  'scale-x-[0.9]',
  'scale-x-[0.95]',
  'scale-x-[1]',
] as const

export function scaleClass(percent: number): string {
  return SCALES[Math.max(0, Math.min(20, Math.round(percent / 5)))] ?? SCALES[0]
}

export const ENTER_DELAYS = [
  '[animation-delay:0ms]',
  '[animation-delay:70ms]',
  '[animation-delay:140ms]',
  '[animation-delay:210ms]',
  '[animation-delay:280ms]',
  '[animation-delay:350ms]',
] as const
