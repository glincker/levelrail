export const LOGIN_CODE_LENGTH = 8

/** normaliseLoginCode strips spaces and hyphens, uppercases and regroups as XXXX-XXXX. */
export function normaliseLoginCode(raw: string): string {
  const clean = raw
    .replace(/[^a-zA-Z0-9]/g, '')
    .toUpperCase()
    .slice(0, LOGIN_CODE_LENGTH)
  return clean.length > 4 ? `${clean.slice(0, 4)}-${clean.slice(4)}` : clean
}
