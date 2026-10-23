import { z } from 'zod'
import { isValidDomain } from './domainWizard'
import { splitDomainInput } from './domainBulk'

const MAX_HOSTNAME_LENGTH = 253

export type DomainIssueReason = 'invalid' | 'duplicate' | 'claimed'

export interface DomainIssue {
  value: string
  reason: DomainIssueReason
  // Owning app when reason is claimed.
  owner?: string
}

export interface DomainListAnalysis {
  domains: string[]
  issues: DomainIssue[]
}

// Parses pasted text into valid unique hostnames plus one issue per bad value.
export function analyzeDomainList(
  text: string,
  claimedBy: ReadonlyMap<string, string> = new Map(),
): DomainListAnalysis {
  const seen = new Set<string>()
  const out: DomainListAnalysis = { domains: [], issues: [] }
  for (const value of splitDomainInput(text)) {
    if (seen.has(value)) {
      out.issues.push({ value, reason: 'duplicate' })
      continue
    }
    seen.add(value)
    if (value.length > MAX_HOSTNAME_LENGTH || !isValidDomain(value)) {
      out.issues.push({ value, reason: 'invalid' })
      continue
    }
    const owner = claimedBy.get(value)
    if (owner !== undefined) {
      out.issues.push({ value, reason: 'claimed', owner })
      continue
    }
    out.domains.push(value)
  }
  return out
}

// Messages are i18n keys under page.addDialog.errors, resolved by the form.
export function createDomainListSchema(
  claimedBy: ReadonlyMap<string, string> = new Map(),
) {
  return z
    .object({
      app: z.string().min(1, 'appRequired'),
      domains: z.string(),
    })
    .superRefine((value, ctx) => {
      const { domains, issues } = analyzeDomainList(value.domains, claimedBy)
      if (issues.length > 0) {
        ctx.addIssue({ code: 'custom', path: ['domains'], message: 'lines' })
      } else if (domains.length === 0) {
        ctx.addIssue({ code: 'custom', path: ['domains'], message: 'empty' })
      }
    })
}

export type DomainListFormValues = z.input<
  ReturnType<typeof createDomainListSchema>
>
