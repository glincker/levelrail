export type CliOrigin = 'pairing' | 'device' | 'password'
export type CliStatus = 'active' | 'expiring' | 'expired'

export interface CliTokenLike {
  name: string
  expires_at?: string | null
  revoked_at?: string | null
  dev_fixture?: boolean
}

const EXPIRING_DAYS = Number(import.meta.env.VITE_CLI_EXPIRING_DAYS ?? 7)
const DAY_MS = 86_400_000

// Every credential a CLI sign-in mints: a pairing code (internal/api/cli_pairing.go),
// the device flow ("device: " + client) and a password login (cmd/levelrail-cli/auth_login.go).
export function cliOrigin(name: string): CliOrigin | null {
  if (name.startsWith('cli-pairing')) {
    return 'pairing'
  }
  if (name.startsWith('device:')) {
    return 'device'
  }
  if (name.startsWith('levelrail-cli-')) {
    return 'password'
  }
  return null
}

export function cliTokenStatus(tok: CliTokenLike, now: number): CliStatus {
  if (!tok.expires_at) {
    return 'active'
  }
  const expires = new Date(tok.expires_at).getTime()
  if (Number.isNaN(expires)) {
    return 'active'
  }
  if (expires <= now) {
    return 'expired'
  }
  return expires - now <= EXPIRING_DAYS * DAY_MS ? 'expiring' : 'active'
}

// Every CLI-origin credential that is not revoked, expired ones included so
// the list can show them as expired.
export function listCliTokens<T extends CliTokenLike>(
  tokens: readonly T[],
): T[] {
  return tokens.filter(
    (tok) =>
      !tok.dev_fixture && cliOrigin(tok.name) !== null && !tok.revoked_at,
  )
}

export function activeCliTokens<T extends CliTokenLike>(
  tokens: readonly T[],
  now: number,
): T[] {
  return listCliTokens(tokens).filter(
    (tok) => cliTokenStatus(tok, now) !== 'expired',
  )
}
