import { readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const tree = JSON.parse(
  readFileSync(path.join(root, 'src/styles/tokens/glinr.tokens.json'), 'utf8'),
)

const TEXT_MIN = 4.5
const UI_MIN = 3

// Reads a token value for a mode; oled falls back to dark like the generated CSS does.
function value(group, key, mode) {
  const v = tree[group][key].$value
  if (typeof v === 'string') {
    const alias = /^\{([^.]+)\.([^}]+)\}$/.exec(v)
    return alias ? value(alias[1], alias[2], mode) : v
  }
  return v[mode] ?? v.dark
}

function parse(css) {
  const m = /^oklch\(([\d.]+) ([\d.]+) ([\d.]+)(?: \/ ([\d.]+)%)?\)$/.exec(css)
  if (!m) throw new Error(`not an oklch colour: ${css}`)
  return {
    L: Number(m[1]),
    C: Number(m[2]),
    h: Number(m[3]),
    a: m[4] === undefined ? 1 : Number(m[4]) / 100,
  }
}

function toLinearRgb({ L, C, h }) {
  const hr = (h * Math.PI) / 180
  const a = C * Math.cos(hr)
  const b = C * Math.sin(hr)
  const l = (L + 0.3963377774 * a + 0.2158037573 * b) ** 3
  const m = (L - 0.1055613458 * a - 0.0638541728 * b) ** 3
  const s = (L - 0.0894841775 * a - 1.291485548 * b) ** 3
  const clamp = (x) => Math.min(1, Math.max(0, x))
  return [
    clamp(4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s),
    clamp(-1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s),
    clamp(-0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s),
  ]
}

function over(fg, bg) {
  const f = parse(fg)
  const lf = toLinearRgb(f)
  const lb = toLinearRgb(parse(bg))
  return lf.map((c, i) => c * f.a + lb[i] * (1 - f.a))
}

function lum([r, g, b]) {
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

function ratio(fg, bg) {
  const a = lum(over(fg, bg)) + 0.05
  const b = lum(toLinearRgb(parse(bg))) + 0.05
  return Math.max(a, b) / Math.min(a, b)
}

// Contrast of fg over a surface, where fg may itself carry alpha and sits on soft over surface.
function ratioOnSoft(fg, soft, surface) {
  const base = toLinearRgb(parse(surface))
  const s = parse(soft)
  const sl = toLinearRgb(s)
  const flat = sl.map((c, i) => c * s.a + base[i] * (1 - s.a))
  const f = toLinearRgb(parse(fg))
  const a = lum(f) + 0.05
  const b = lum(flat) + 0.05
  return Math.max(a, b) / Math.min(a, b)
}

const TONES = ['success', 'warning', 'danger', 'info', 'neutral']

describe.each(['light', 'dark', 'oled'])('token contrast, %s', (mode) => {
  const ui = (k) => value('ui', k, mode)
  const raised = ui('surface-raised')
  const bg = ui('surface-bg')

  it('keeps body text at 4.5 or more', () => {
    for (const surface of [bg, raised, ui('surface-sunken')]) {
      expect(ratio(value('color', 'fg', mode), surface)).toBeGreaterThanOrEqual(TEXT_MIN)
      expect(ratio(value('color', 'fg-muted', mode), surface)).toBeGreaterThanOrEqual(TEXT_MIN)
    }
    expect(ratio(ui('accent-fg'), raised)).toBeGreaterThanOrEqual(TEXT_MIN)
    for (const k of ['accent-solid', 'accent-solid-hover', 'accent-solid-active']) {
      expect(ratio(ui('accent-ink'), ui(k))).toBeGreaterThanOrEqual(TEXT_MIN)
    }
  })

  it('keeps UI edges, dots and the focus ring at 3 or more', () => {
    expect(ratio(ui('border-control'), raised)).toBeGreaterThanOrEqual(UI_MIN)
    expect(ratio(ui('accent-ring'), raised)).toBeGreaterThanOrEqual(UI_MIN)
    expect(ratio(ui('accent-solid'), raised)).toBeGreaterThanOrEqual(UI_MIN)
    for (const tone of TONES) {
      expect(ratio(value('status', `${tone}-solid`, mode), raised)).toBeGreaterThanOrEqual(UI_MIN)
    }
  })

  it('keeps status text readable on its soft fill', () => {
    for (const tone of TONES) {
      const fg = value('status', `${tone}-fg`, mode)
      const soft = value('status', `${tone}-soft`, mode)
      expect(ratioOnSoft(fg, soft, raised), tone).toBeGreaterThanOrEqual(
        TEXT_MIN,
      )
    }
  })
})
