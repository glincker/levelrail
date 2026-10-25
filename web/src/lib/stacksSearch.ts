// URL search for /settings/stacks: which stack, revision and diff base is open.
export interface StacksSearch {
  stack?: string
  rev?: number
  diff?: boolean
  against?: string
}

export function parseStacksSearch(raw: Record<string, unknown>): StacksSearch {
  const rev = Number(raw.rev)
  return {
    stack: typeof raw.stack === 'string' && raw.stack ? raw.stack : undefined,
    rev: Number.isInteger(rev) && rev > 0 ? rev : undefined,
    diff: raw.diff === true || raw.diff === 'true' ? true : undefined,
    against:
      typeof raw.against === 'string' || typeof raw.against === 'number'
        ? String(raw.against)
        : undefined,
  }
}
