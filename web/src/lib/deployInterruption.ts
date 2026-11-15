export type DeployInterruption = 'interrupted' | 'orphaned'

// The server writes these reasons as "<word>: <detail>", so the prefix is the stable part.
export function deployInterruption(
  reason: string | null | undefined,
): DeployInterruption | null {
  if (!reason) return null
  if (reason.startsWith('interrupted:')) return 'interrupted'
  if (reason.startsWith('orphaned:')) return 'orphaned'
  return null
}
