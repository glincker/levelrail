import type { AdvisorFinding } from '../types/resourceAdvisor'

/** Idle apps the bulk sleep action can enable right now. */
export function sleepableFindings(
  findings: AdvisorFinding[],
): AdvisorFinding[] {
  return findings.filter(
    (f) =>
      f.kind === 'idle_app' &&
      f.appliable &&
      f.can_apply &&
      f.state === undefined,
  )
}

export function totalReclaimableMb(findings: AdvisorFinding[]): number {
  return findings.reduce((sum, f) => sum + (f.reclaimable_mb ?? 0), 0)
}

/** Sum of known monthly savings, or null when no finding has a price (never zero). */
export function totalMonthlySaving(findings: AdvisorFinding[]): number | null {
  const known = findings
    .map((f) => f.impact.monthly_saving)
    .filter((v): v is number => v !== undefined)
  return known.length === 0 ? null : known.reduce((a, b) => a + b, 0)
}

export interface NodePriceRow {
  node: string
  price: string
}

/** Reads the advisor.node_prices text ("node=amount,node=amount") into rows. */
export function parseNodePrices(text: string): NodePriceRow[] {
  return text
    .split(',')
    .map((part) => part.trim())
    .filter((part) => part !== '')
    .map((part) => {
      const [node = '', price = ''] = part.split('=')
      return { node: node.trim(), price: price.trim() }
    })
}

/** Returns the setting text, or null when a row is not a name with a positive amount. */
export function formatNodePrices(rows: NodePriceRow[]): string | null {
  const parts: string[] = []
  for (const r of rows) {
    if (r.node === '' && r.price === '') continue
    const amount = Number(r.price)
    if (!/^[^,=\s]+$/.test(r.node) || !Number.isFinite(amount) || amount <= 0) {
      return null
    }
    parts.push(`${r.node}=${amount}`)
  }
  return parts.join(',')
}
