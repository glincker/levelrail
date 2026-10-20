export function formatRelative(
  at: string | Date,
  now: number = Date.now(),
): string {
  const ms = at instanceof Date ? at.getTime() : Date.parse(at)
  if (!Number.isFinite(ms)) return ''
  const diff = Math.round((now - ms) / 1000)
  if (diff < 0) {
    const ahead = -diff
    if (ahead < 45) return 'in a moment'
    if (ahead < 3600) return `in ${Math.round(ahead / 60)}m`
    if (ahead < 86400) return `in ${Math.round(ahead / 3600)}h`
    return `in ${Math.round(ahead / 86400)}d`
  }
  if (diff < 45) return 'just now'
  if (diff < 3600) return `${Math.max(1, Math.round(diff / 60))}m ago`
  if (diff < 86400) return `${Math.round(diff / 3600)}h ago`
  if (diff < 86400 * 30) return `${Math.round(diff / 86400)}d ago`
  if (diff < 86400 * 365) return `${Math.round(diff / (86400 * 30))}mo ago`
  return `${Math.round(diff / (86400 * 365))}y ago`
}
