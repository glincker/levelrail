// Message the zod schema attaches; the field component maps it to copy.
export const publicPortError = 'invalid-public-port'

export function isValidPublicPort(raw: string): boolean {
  if (raw === '') return true
  if (!/^\d{1,5}$/.test(raw)) return false
  return Number(raw) <= 65535
}
