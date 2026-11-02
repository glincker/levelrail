import { useSyncExternalStore } from 'react'
import { setDensity, type Density } from './density'
import { setPageWidth, type PageWidth } from './pageWidth'

// Browser-only store behind one hook. Swap this module for an account
// preferences query when server-side storage lands; callers do not change.

export const DARK_BASES = ['soft', 'black', 'graphite', 'midnight'] as const
export const LIGHT_BASES = ['white', 'grey', 'paper', 'cool'] as const
export const FONTS = ['system', 'rounded', 'serif', 'mono', 'readable'] as const
export const TEXT_SIZES = ['90', '100', '110'] as const
export const ACCENTS = [
  'teal',
  'indigo',
  'violet',
  'green',
  'amber',
  'rose',
] as const
export const GUTTERS = ['tight', 'normal', 'roomy'] as const

export type DarkBase = (typeof DARK_BASES)[number]
export type LightBase = (typeof LIGHT_BASES)[number]
export type FontChoice = (typeof FONTS)[number]
export type TextSize = (typeof TEXT_SIZES)[number]
export type Accent = (typeof ACCENTS)[number]
export type Gutter = (typeof GUTTERS)[number]

export interface Appearance {
  darkBase: DarkBase
  lightBase: LightBase
  font: FontChoice
  textSize: TextSize
  gutter: Gutter
  accent: Accent
}

export const APPEARANCE_DEFAULTS: Appearance = {
  darkBase: 'soft',
  lightBase: 'white',
  font: 'system',
  textSize: '100',
  gutter: 'normal',
  accent: 'teal',
}

// Where the choice differs from the default, these mark the recommended pick.
export const APPEARANCE_RECOMMENDED = {
  darkBase: 'graphite',
  lightBase: 'grey',
  font: 'readable',
  textSize: '100',
  width: 'comfortable',
  gutter: 'normal',
  accent: 'teal',
  density: 'compact',
} as const

export const STORAGE_KEY = 'levelrail-appearance-v1'

const ATTRS: Record<keyof Appearance, string> = {
  darkBase: 'data-dark-base',
  lightBase: 'data-light-base',
  font: 'data-font',
  textSize: 'data-text-size',
  gutter: 'data-gutter',
  accent: 'data-accent',
}

function pick<T extends string>(
  options: readonly T[],
  value: unknown,
  fallback: T,
): T {
  return options.find((o) => o === value) ?? fallback
}

/** parseAppearance falls back per field, so one bad stored value never drops the rest. */
export function parseAppearance(raw: string | null): Appearance {
  if (!raw) return { ...APPEARANCE_DEFAULTS }
  let data: Record<string, unknown> = {}
  try {
    const parsed: unknown = JSON.parse(raw)
    if (parsed && typeof parsed === 'object') {
      data = parsed as Record<string, unknown>
    }
  } catch {
    return { ...APPEARANCE_DEFAULTS }
  }
  const d = APPEARANCE_DEFAULTS
  return {
    darkBase: pick(DARK_BASES, data.darkBase, d.darkBase),
    lightBase: pick(LIGHT_BASES, data.lightBase, d.lightBase),
    font: pick(FONTS, data.font, d.font),
    textSize: pick(TEXT_SIZES, data.textSize, d.textSize),
    gutter: pick(GUTTERS, data.gutter, d.gutter),
    accent: pick(ACCENTS, data.accent, d.accent),
  }
}

function read(): Appearance {
  try {
    return parseAppearance(localStorage.getItem(STORAGE_KEY))
  } catch {
    return { ...APPEARANCE_DEFAULTS }
  }
}

/** applyAppearance stamps data attributes on <html>; defaults clear them so the stock theme stays untouched. */
export function applyAppearance(value: Appearance) {
  if (typeof document === 'undefined') return
  const root = document.documentElement
  for (const key of Object.keys(ATTRS) as (keyof Appearance)[]) {
    const attr = ATTRS[key]
    if (value[key] === APPEARANCE_DEFAULTS[key]) root.removeAttribute(attr)
    else root.setAttribute(attr, value[key])
  }
}

const listeners = new Set<() => void>()
let current: Appearance = read()
applyAppearance(current)

export function getAppearance(): Appearance {
  return current
}

export function setAppearance(patch: Partial<Appearance>) {
  current = { ...current, ...patch }
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(current))
  } catch {
    // Storage can be blocked; the in-memory choice still applies this session.
  }
  applyAppearance(current)
  listeners.forEach((l) => l())
}

export const PRESETS = ['default', 'focusDark', 'oled', 'paper'] as const
export type PresetId = (typeof PRESETS)[number]

export interface Preset {
  appearance: Appearance
  width: PageWidth
  density: Density
  /** Color mode to switch to so the chosen base is the one on screen. */
  theme?: 'light' | 'dark'
}

const D = APPEARANCE_DEFAULTS

export const PRESET_VALUES: Record<PresetId, Preset> = {
  default: { appearance: D, width: 'comfortable', density: 'compact' },
  focusDark: {
    appearance: { ...D, darkBase: 'graphite', gutter: 'roomy' },
    width: 'comfortable',
    density: 'comfortable',
    theme: 'dark',
  },
  oled: {
    appearance: { ...D, darkBase: 'black' },
    width: 'wide',
    density: 'compact',
    theme: 'dark',
  },
  paper: {
    appearance: { ...D, lightBase: 'paper', font: 'readable', gutter: 'roomy' },
    width: 'compact',
    density: 'comfortable',
    theme: 'light',
  },
}

/** applyPreset sets every appearance option at once and returns the preset so the caller can switch theme. */
export function applyPreset(id: PresetId): Preset {
  const preset = PRESET_VALUES[id]
  setAppearance(preset.appearance)
  setPageWidth(preset.width)
  setDensity(preset.density)
  return preset
}

export function resetAppearance() {
  applyPreset('default')
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

export function useAppearance(): Appearance {
  return useSyncExternalStore(subscribe, getAppearance, getAppearance)
}
