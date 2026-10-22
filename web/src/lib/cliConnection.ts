// A CLI pairing mints an owned token named "cli-pairing", optionally
// followed by a client name (internal/api/cli_pairing.go).
export const CLI_TOKEN_PREFIX = 'cli-pairing'

export interface CliTokenLike {
  name: string
  expires_at?: string | null
  revoked_at?: string | null
}

export function activeCliTokens<T extends CliTokenLike>(
  tokens: readonly T[],
  now: number,
): T[] {
  return tokens.filter((tok) => {
    if (!tok.name.startsWith(CLI_TOKEN_PREFIX)) {
      return false
    }
    if (tok.revoked_at) {
      return false
    }
    if (tok.expires_at) {
      const expires = new Date(tok.expires_at).getTime()
      if (!Number.isNaN(expires) && expires <= now) {
        return false
      }
    }
    return true
  })
}

export function isCliConnected(
  tokens: readonly CliTokenLike[],
  now: number,
): boolean {
  return activeCliTokens(tokens, now).length > 0
}
