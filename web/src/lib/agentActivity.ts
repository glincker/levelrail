import type { AuditLogEntry } from '../queries/auditLog'

const APPROVAL_PATH = /\/deploy-approvals\/([^/]+)\/(approve|reject)$/

// Anything not typed by a signed-in person: tokens, MCP and API clients,
// labeled agents and AI turns acting on someone's behalf.
export function isNonHumanEntry(e: AuditLogEntry): boolean {
  return (
    e.actor_type === 'token' ||
    e.client_kind === 'mcp' ||
    e.client_kind === 'api' ||
    Boolean(e.agent_name) ||
    Boolean(e.on_behalf_of)
  )
}

export function approvalIdOf(
  e: Pick<AuditLogEntry, 'path'>,
): string | undefined {
  return APPROVAL_PATH.exec(e.path)?.[1]
}
