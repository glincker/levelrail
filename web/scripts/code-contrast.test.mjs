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

const SYNTAX = [
  'fg',
  'muted',
  'keyword',
  'string',
  'number',
  'function',
  'type',
  'property',
  'comment',
  'punctuation',
  'added',
  'removed',
]
const ANSI = [
  'black',
  'red',
  'green',
  'yellow',
  'blue',
  'magenta',
  'cyan',
  'white',
]
const ANSI_KEYS = ANSI.flatMap((n) => [`ansi-${n}`, `ansi-bright-${n}`])
const LEVELS = [
  'log-info',
  'log-warn',
  'log-error',
  'log-success',
  'log-debug',
  'log-timestamp',
]

function code(key, mode) {
  return tree.code[key].$value[mode]
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

// Flattens a possibly translucent colour over an opaque linear base.
function flatten(css, base) {
  const p = parse(css)
  return toLinearRgb(p).map((c, i) => c * p.a + base[i] * (1 - p.a))
}

function lum([r, g, b]) {
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

function ratio(fgCss, surfaceLinear) {
  const a = lum(flatten(fgCss, surfaceLinear)) + 0.05
  const b = lum(surfaceLinear) + 0.05
  return Math.max(a, b) / Math.min(a, b)
}

describe.each(['light', 'dark'])('code palette contrast (%s)', (mode) => {
  const bg = toLinearRgb(parse(code('bg', mode)))
  const onTint = (tint) => flatten(code(tint, mode), bg)

  it.each([...SYNTAX, ...ANSI_KEYS, ...LEVELS])(
    '%s is at least 4.5:1 on code-bg',
    (key) => {
      expect(ratio(code(key, mode), bg)).toBeGreaterThanOrEqual(TEXT_MIN)
    },
  )

  it.each(['selection', 'line-highlight', 'match'])(
    'fg stays readable on %s',
    (tint) => {
      expect(ratio(code('fg', mode), onTint(tint))).toBeGreaterThanOrEqual(
        TEXT_MIN,
      )
    },
  )

  it('muted text stays readable on the hovered line', () => {
    expect(
      ratio(code('muted', mode), onTint('line-highlight')),
    ).toBeGreaterThanOrEqual(TEXT_MIN)
  })

  it('diff text stays readable on its own tinted background', () => {
    expect(
      ratio(code('added', mode), onTint('added-bg')),
    ).toBeGreaterThanOrEqual(TEXT_MIN)
    expect(
      ratio(code('removed', mode), onTint('removed-bg')),
    ).toBeGreaterThanOrEqual(TEXT_MIN)
    expect(ratio(code('fg', mode), onTint('added-bg'))).toBeGreaterThanOrEqual(
      TEXT_MIN,
    )
    expect(
      ratio(code('fg', mode), onTint('removed-bg')),
    ).toBeGreaterThanOrEqual(TEXT_MIN)
  })

  it('level text stays readable on the error and warning line tints', () => {
    for (const tint of ['log-error-bg', 'log-warn-bg']) {
      for (const key of ['fg', 'log-error', 'log-warn', 'log-timestamp']) {
        expect(ratio(code(key, mode), onTint(tint))).toBeGreaterThanOrEqual(
          TEXT_MIN,
        )
      }
    }
  })

  it('the error gutter bar is a visible UI element', () => {
    expect(ratio(code('log-error-border', mode), bg)).toBeGreaterThanOrEqual(
      UI_MIN,
    )
  })
})
