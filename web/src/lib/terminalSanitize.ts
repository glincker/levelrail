export interface SanitizeState {
  pending: string
  inString: boolean
  stringIsOsc: boolean
}

export const EMPTY_SANITIZE_STATE: SanitizeState = {
  pending: '',
  inString: false,
  stringIsOsc: false,
}

const ESC = '\x1b'
const BEL = '\x07'
const MAX_PENDING = 64
const KEPT_C0 = new Set(['\n', '\r', '\t', '\b'])
// SGR plus harmless erase-line and cursor moves; every other CSI is dropped.
const ALLOWED_CSI_FINALS = new Set(['m', 'K', 'A', 'B', 'C', 'D'])
const STRING_INTRODUCERS = new Set([']', 'P', '_', '^', 'X'])
const SIMPLE_PARAMS = /^[0-9;:]*$/

function isCsiParam(c: string): boolean {
  return c >= '0' && c <= '?'
}

function isIntermediate(c: string): boolean {
  return c >= ' ' && c <= '/'
}

function isFinal(c: string): boolean {
  return c >= '@' && c <= '~'
}

function carryOver(
  out: string,
  rest: string,
): {
  text: string
  state: SanitizeState
} {
  if (rest.length > MAX_PENDING) {
    return { text: out, state: EMPTY_SANITIZE_STATE }
  }
  return {
    text: out,
    state: { pending: rest, inString: false, stringIsOsc: false },
  }
}

// Returns the safe text to write and the state to pass with the next chunk,
// so a sequence split across websocket frames is never half-emitted.
export function sanitizeTerminalOutput(
  input: string,
  state: SanitizeState = EMPTY_SANITIZE_STATE,
): { text: string; state: SanitizeState } {
  const src = state.pending + input
  let out = ''
  let inString = state.inString
  let stringIsOsc = state.stringIsOsc
  let i = 0

  while (i < src.length) {
    const c = src.charAt(i)

    if (inString) {
      if (stringIsOsc && c === BEL) {
        inString = false
        i++
      } else if (c === ESC) {
        if (i + 1 >= src.length) {
          return { text: out, state: { pending: ESC, inString, stringIsOsc } }
        }
        inString = false
        i += src.charAt(i + 1) === '\\' ? 2 : 0
      } else {
        i++
      }
      continue
    }

    if (c === ESC) {
      if (i + 1 >= src.length) return carryOver(out, ESC)
      const next = src.charAt(i + 1)
      if (STRING_INTRODUCERS.has(next)) {
        inString = true
        stringIsOsc = next === ']'
        i += 2
        continue
      }
      let j = i + 2
      if (next === '[') {
        while (j < src.length && isCsiParam(src.charAt(j))) j++
      } else {
        j = i + 1
      }
      while (j < src.length && isIntermediate(src.charAt(j))) j++
      if (j >= src.length) return carryOver(out, src.slice(i))
      if (isFinal(src.charAt(j))) {
        const seq = src.slice(i, j + 1)
        if (
          next === '[' &&
          ALLOWED_CSI_FINALS.has(src.charAt(j)) &&
          SIMPLE_PARAMS.test(src.slice(i + 2, j))
        ) {
          out += seq
        }
        i = j + 1
      } else {
        i = j
      }
      continue
    }

    const code = c.charCodeAt(0)
    if (code < 0x20) {
      if (KEPT_C0.has(c)) out += c
    } else if (code !== 0x7f && (code < 0x80 || code > 0x9f)) {
      out += c
    }
    i++
  }

  return { text: out, state: { pending: '', inString, stringIsOsc } }
}

export function sanitizeTerminalText(input: string): string {
  return sanitizeTerminalOutput(input).text
}
