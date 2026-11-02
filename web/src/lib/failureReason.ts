import type { AppStatusSummary } from '../types/appDetail'

// Shows why a failing app needs attention; the full text rides in the title.
export function failureReason(status: AppStatusSummary): string | null {
  if (status.variant !== 'destructive') return null
  const text = status.message?.trim() || status.reason?.trim()
  if (!text) return null
  return text
}
