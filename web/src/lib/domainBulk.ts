import { isValidDomain } from './domainWizard'

export interface ParsedDomains {
  // Valid, unclaimed and not repeated: safe to submit.
  add: string[]
  invalid: string[]
  // Same hostname typed more than once in the input.
  duplicates: string[]
  // Already routed: app is the owning app's name.
  claimed: { domain: string; app: string }[]
}

// Accepts pasted lists: newline, comma, semicolon or whitespace separated,
// with or without a scheme or path.
export function splitDomainInput(text: string): string[] {
  return text
    .split(/[\s,;]+/)
    .map((token) =>
      token
        .trim()
        .toLowerCase()
        .replace(/^[a-z]+:\/\//, '')
        .replace(/\/.*$/, '')
        .replace(/\.$/, ''),
    )
    .filter((token) => token !== '')
}

export function parseDomainInput(
  text: string,
  claimedBy: ReadonlyMap<string, string> = new Map(),
): ParsedDomains {
  const seen = new Set<string>()
  const out: ParsedDomains = {
    add: [],
    invalid: [],
    duplicates: [],
    claimed: [],
  }
  for (const domain of splitDomainInput(text)) {
    if (seen.has(domain)) {
      if (!out.duplicates.includes(domain)) out.duplicates.push(domain)
      continue
    }
    seen.add(domain)
    if (!isValidDomain(domain)) {
      out.invalid.push(domain)
      continue
    }
    const owner = claimedBy.get(domain)
    if (owner !== undefined) {
      out.claimed.push({ domain, app: owner })
      continue
    }
    out.add.push(domain)
  }
  return out
}

export type BulkAddOutcome<T = unknown> =
  | { domain: string; ok: true; result: T }
  | { domain: string; ok: false; error: string }

// One call per domain so a rejected hostname never blocks the others.
export async function addDomainsIndependently<T = unknown>(
  domains: readonly string[],
  addOne: (domain: string) => Promise<T>,
): Promise<BulkAddOutcome<T>[]> {
  const outcomes: BulkAddOutcome<T>[] = []
  for (const domain of domains) {
    try {
      const result = await addOne(domain)
      outcomes.push({ domain, ok: true, result })
    } catch (error) {
      outcomes.push({
        domain,
        ok: false,
        error: error instanceof Error ? error.message : String(error),
      })
    }
  }
  return outcomes
}
