import qrcode from 'qrcode-generator'

// Default bytes mode drops non-Latin characters; the UTF-8 encoder keeps them.
const utf8 = qrcode.stringToBytesFuncs['UTF-8']
if (utf8) qrcode.stringToBytes = utf8

/** Quiet zone in modules, drawn inside the light tile so scanners find the code. */
export const QR_QUIET = 4
export const FINDER = 7
const DOT = 0.5

/** Percent of the brand accent kept when darkening module color for scanner contrast. */
export const QR_ACCENT_PERCENT = 55

export const MODULE_FILL = `fill-[color-mix(in_srgb,var(--brand-accent)_${String(QR_ACCENT_PERCENT)}%,black)]`

export interface QrModel {
  size: number
  dark: boolean[][]
}

export function buildQrModel(value: string): QrModel {
  const qr = qrcode(0, 'Q')
  qr.addData(value)
  qr.make()
  const size = qr.getModuleCount()
  const dark = Array.from({ length: size }, (_, r) =>
    Array.from({ length: size }, (_, c) => qr.isDark(r, c)),
  )
  return { size, dark }
}

export function finderOrigins(size: number): [number, number][] {
  return [
    [0, 0],
    [size - FINDER, 0],
    [0, size - FINDER],
  ]
}

function inFinder(x: number, y: number, size: number): boolean {
  return finderOrigins(size).some(
    ([fx, fy]) => x >= fx && x < fx + FINDER && y >= fy && y < fy + FINDER,
  )
}

/** One module as a rounded shape: only corners with no neighbor on either side are round, so runs join smoothly. */
function modulePath(model: QrModel, x: number, y: number): string {
  const on = (cx: number, cy: number) =>
    cx >= 0 &&
    cy >= 0 &&
    cx < model.size &&
    cy < model.size &&
    model.dark[cy]?.[cx] === true &&
    !inFinder(cx, cy, model.size)
  const up = on(x, y - 1)
  const down = on(x, y + 1)
  const left = on(x - 1, y)
  const right = on(x + 1, y)
  const tl = up || left ? 0 : DOT
  const tr = up || right ? 0 : DOT
  const br = down || right ? 0 : DOT
  const bl = down || left ? 0 : DOT
  const arc = (r: number, ex: number, ey: number) =>
    r > 0 ? `A${r} ${r} 0 0 1 ${ex} ${ey}` : ''
  return [
    `M${x} ${y + 0.5}`,
    `V${y + tl}`,
    arc(tl, x + tl, y),
    `H${x + 1 - tr}`,
    arc(tr, x + 1, y + tr),
    `V${y + 1 - br}`,
    arc(br, x + 1 - br, y + 1),
    `H${x + bl}`,
    arc(bl, x, y + 1 - bl),
    'Z',
  ].join('')
}

/** Path of every data module in module units, finder areas excluded. */
export function dataPath(model: QrModel): string {
  const parts: string[] = []
  for (let y = 0; y < model.size; y++) {
    for (let x = 0; x < model.size; x++) {
      if (model.dark[y]?.[x] && !inFinder(x, y, model.size)) {
        parts.push(modulePath(model, x, y))
      }
    }
  }
  return parts.join('')
}
