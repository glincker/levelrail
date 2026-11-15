// Maps SGR foreground codes to the --glinr-code-ansi-* tokens. Rendering still strips
// escapes (see ansi.ts); this is the single place a coloured renderer would read from.
const NAMES = [
  'black',
  'red',
  'green',
  'yellow',
  'blue',
  'magenta',
  'cyan',
  'white',
] as const

export function ansiColorVar(sgr: number): string | null {
  if (sgr >= 30 && sgr <= 37) return `--glinr-code-ansi-${NAMES[sgr - 30]}`
  if (sgr >= 90 && sgr <= 97) {
    return `--glinr-code-ansi-bright-${NAMES[sgr - 90]}`
  }
  return null
}

// Literal class names so Tailwind generates the utilities.
export const ANSI_TEXT_CLASS: Record<string, string> = {
  black: 'text-glinr-code-ansi-black',
  red: 'text-glinr-code-ansi-red',
  green: 'text-glinr-code-ansi-green',
  yellow: 'text-glinr-code-ansi-yellow',
  blue: 'text-glinr-code-ansi-blue',
  magenta: 'text-glinr-code-ansi-magenta',
  cyan: 'text-glinr-code-ansi-cyan',
  white: 'text-glinr-code-ansi-white',
  'bright-black': 'text-glinr-code-ansi-bright-black',
  'bright-red': 'text-glinr-code-ansi-bright-red',
  'bright-green': 'text-glinr-code-ansi-bright-green',
  'bright-yellow': 'text-glinr-code-ansi-bright-yellow',
  'bright-blue': 'text-glinr-code-ansi-bright-blue',
  'bright-magenta': 'text-glinr-code-ansi-bright-magenta',
  'bright-cyan': 'text-glinr-code-ansi-bright-cyan',
  'bright-white': 'text-glinr-code-ansi-bright-white',
}
