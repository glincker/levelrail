import type { AuditLogEntry } from '../queries/auditLog'

export function filterAuditEntries(
  entries: AuditLogEntry[],
  text: string,
  failedOnly: boolean,
): AuditLogEntry[] {
  const needle = text.trim().toLowerCase()
  if (needle === '' && !failedOnly) {
    return entries
  }
  return entries.filter((e) => {
    if (failedOnly && e.status_code < 400) {
      return false
    }
    return (
      needle === '' ||
      [e.actor_name, e.ability, e.method, e.path, e.remote_addr]
        .join(' ')
        .toLowerCase()
        .includes(needle)
    )
  })
}
