// Pure color math for the custom accent: sRGB and OKLCH conversion, WCAG
// contrast, and a lightness nudge so a chosen accent is always readable.

export type Rgb = readonly [number, number, number]
export type Oklch = readonly [number, number, number]

export const MIN_TEXT_CONTRAST = 4.5
const NUDGE_STEP = 0.005
const HEX_RE = /^#?([0-9a-f]{6})$/i

/** normalizeHex returns lowercase #rrggbb, or null when the input is not a six digit hex color. */
export function normalizeHex(input: string): string | null {
  const m = HEX_RE.exec(input.trim())
  return m ? `#${(m[1] ?? '').toLowerCase()}` : null
}

export function hexToRgb(hex: string): Rgb {
  const n = normalizeHex(hex) ?? '#000000'
  const v = parseInt(n.slice(1), 16)
  return [((v >> 16) & 255) / 255, ((v >> 8) & 255) / 255, (v & 255) / 255]
}

const toLinear = (c: number) =>
  c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
const fromLinear = (c: number) =>
  c <= 0.0031308 ? 12.92 * c : 1.055 * c ** (1 / 2.4) - 0.055

export function rgbToOklch(rgb: Rgb): Oklch {
  const [r, g, b] = rgb.map(toLinear) as [number, number, number]
  const l = Math.cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b)
  const m = Math.cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b)
  const s = Math.cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b)
  const L = 0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s
  const a = 1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s
  const bb = 0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s
  const C = Math.hypot(a, bb)
  const h = C < 1e-4 ? 0 : ((Math.atan2(bb, a) * 180) / Math.PI + 360) % 360
  return [L, C, h]
}

function oklchToLinear(L: number, C: number, h: number): number[] {
  const a = C * Math.cos((h * Math.PI) / 180)
  const b = C * Math.sin((h * Math.PI) / 180)
  const l = (L + 0.3963377774 * a + 0.2158037573 * b) ** 3
  const m = (L - 0.1055613458 * a - 0.0638541728 * b) ** 3
  const s = (L - 0.0894841775 * a - 1.291485548 * b) ** 3
  return [
    4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
    -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
    -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s,
  ]
}

const inGamut = (lin: number[]) => lin.every((c) => c >= -0.0005 && c <= 1.0005)

/** oklchToRgb lowers chroma until the color fits sRGB, so lightness (and contrast) is preserved. */
export function oklchToRgb(color: Oklch): Rgb {
  const [L, C, h] = color
  let c = C
  let lin = oklchToLinear(L, c, h)
  for (let i = 0; i < 40 && !inGamut(lin); i++) {
    c *= 0.93
    lin = oklchToLinear(L, c, h)
  }
  const [r, g, b] = lin.map((x) => fromLinear(Math.min(1, Math.max(0, x))))
  return [r ?? 0, g ?? 0, b ?? 0]
}

function luminance(rgb: Rgb): number {
  const [r, g, b] = rgb.map(toLinear) as [number, number, number]
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

/** contrastRatio is the WCAG 2 ratio, 1 to 21. */
export function contrastRatio(a: Rgb, b: Rgb): number {
  const la = luminance(a)
  const lb = luminance(b)
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05)
}

/** nudgeLightness moves L in one direction until the color passes against every reference, or reaches black or white. */
export function nudgeLightness(
  color: Oklch,
  against: readonly Rgb[],
  direction: 'darker' | 'lighter',
  min = MIN_TEXT_CONTRAST,
): { color: Oklch; adjusted: boolean } {
  const passes = (c: Oklch) => {
    const rgb = oklchToRgb(c)
    return against.every((ref) => contrastRatio(rgb, ref) >= min)
  }
  if (passes(color)) return { color, adjusted: false }
  const sign = direction === 'darker' ? -1 : 1
  let L = color[0]
  for (let i = 0; i < 400; i++) {
    L = Math.min(1, Math.max(0, L + sign * NUDGE_STEP))
    const next: Oklch = [L, color[1], color[2]]
    if (passes(next) || L === 0 || L === 1)
      return { color: next, adjusted: true }
  }
  return { color: [L, color[1], color[2]], adjusted: true }
}

// Text on a solid accent: white in light mode, near black in dark mode.
const LIGHT_ACCENT_FG: Rgb = [1, 1, 1]
const DARK_ACCENT_FG: Rgb = oklchToRgb([0.15, 0, 0])

export interface DerivedAccent {
  light: string
  dark: string
  adjusted: boolean
}

const css = ([L, C, h]: Oklch) =>
  `oklch(${L.toFixed(3)} ${C.toFixed(3)} ${h.toFixed(1)})`

/** deriveAccent builds readable light and dark variants of a hex color against the given backgrounds. */
export function deriveAccent(
  hex: string,
  lightBg: Oklch,
  darkBg: Oklch,
): DerivedAccent {
  const base = rgbToOklch(hexToRgb(hex))
  const light = nudgeLightness(
    base,
    [oklchToRgb(lightBg), LIGHT_ACCENT_FG],
    'darker',
  )
  const dark = nudgeLightness(
    base,
    [oklchToRgb(darkBg), DARK_ACCENT_FG],
    'lighter',
  )
  return {
    light: css(light.color),
    dark: css(dark.color),
    adjusted: light.adjusted || dark.adjusted,
  }
}

/** Page background of each base, mirroring the index.css base blocks. */
export const BASE_BACKGROUND: Record<string, Oklch> = {
  soft: [0.145, 0, 0],
  black: [0, 0, 0],
  graphite: [0.2, 0.006, 260],
  midnight: [0.16, 0.035, 265],
  white: [1, 0, 0],
  grey: [0.955, 0, 0],
  paper: [0.965, 0.016, 85],
  cool: [0.95, 0.01, 250],
}
