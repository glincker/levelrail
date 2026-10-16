// formatBuildSeconds renders a build duration as "850ms", "42s" or "3m 05s".
export function formatBuildSeconds(seconds: number): string {
  if (seconds < 1) return `${Math.round(seconds * 1000)}ms`
  if (seconds < 60) return `${Math.round(seconds)}s`
  const m = Math.floor(seconds / 60)
  const s = Math.round(seconds - m * 60)
  return `${m}m ${String(s).padStart(2, '0')}s`
}
