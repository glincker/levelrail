import type { AuditLogEntry } from '../queries/auditLog'
import type { TokenResource } from '../types/token'

// Distinct agent names known from token labels and loaded audit entries,
// sorted, with the active filter kept so its chip never disappears.
export function collectAgentNames(
  tokens: TokenResource[],
  entries: AuditLogEntry[],
  active?: string,
): string[] {
  const names = new Set<string>()
  for (const token of tokens) {
    if (token.agent?.name) names.add(token.agent.name)
  }
  for (const entry of entries) {
    if (entry.agent_name) names.add(entry.agent_name)
  }
  if (active) names.add(active)
  return [...names].sort((a, b) => a.localeCompare(b))
}
