export const MIN_PASSWORD_LENGTH = 8

export type PasswordScore = 0 | 1 | 2 | 3 | 4

export interface PasswordRequirements {
  length: boolean
  mixedCase: boolean
  digit: boolean
  symbol: boolean
}

const COMMON = [
  'password',
  'passw0rd',
  'qwerty',
  'letmein',
  'welcome',
  'admin',
  '123456',
  '111111',
  'iloveyou',
]

export function passwordRequirements(pw: string): PasswordRequirements {
  return {
    length: pw.length >= MIN_PASSWORD_LENGTH,
    mixedCase: /[a-z]/.test(pw) && /[A-Z]/.test(pw),
    digit: /\d/.test(pw),
    symbol: /[^A-Za-z0-9]/.test(pw),
  }
}

function hasRun(pw: string): boolean {
  return (
    /(.)\1{3,}/.test(pw) ||
    /(0123|1234|2345|3456|4567|5678|6789|abcd|bcde|cdef)/i.test(pw)
  )
}

/** scorePassword is a dependency-free heuristic: length and variety, minus common and repetitive patterns. */
export function scorePassword(pw: string): PasswordScore {
  if (pw.length === 0) return 0
  if (pw.length < MIN_PASSWORD_LENGTH) return 1
  const lower = pw.toLowerCase()
  if (COMMON.some((c) => lower.includes(c))) return 1
  const req = passwordRequirements(pw)
  let points = [req.mixedCase, req.digit, req.symbol].filter(Boolean).length
  if (pw.length >= 12) points += 1
  if (pw.length >= 16) points += 1
  if (new Set(pw).size < pw.length / 2) points -= 1
  if (hasRun(pw)) points -= 1
  if (points <= 1) return 1
  if (points === 2) return 2
  if (points === 3) return 3
  return 4
}
