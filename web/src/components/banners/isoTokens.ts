export type Palette = { top: string; l: string; r: string }

export const BLUE: Palette = {
  top: 'fill-(--iso-top)',
  l: 'fill-(--iso-l)',
  r: 'fill-(--iso-r)',
}
export const GREY: Palette = {
  top: 'fill-(--iso-sleep-top)',
  l: 'fill-(--iso-sleep-l)',
  r: 'fill-(--iso-sleep-r)',
}
export const FLOOR: Palette = {
  top: 'fill-(--iso-floor-top)',
  l: 'fill-(--iso-floor-l)',
  r: 'fill-(--iso-floor-r)',
}
export const YELLOW: Palette = {
  top: 'fill-(--iso-accent-top)',
  l: 'fill-(--iso-accent-l)',
  r: 'fill-(--iso-accent-r)',
}

export const LABEL =
  'fill-muted-foreground font-mono text-[14px] uppercase tracking-[0.06em]'
