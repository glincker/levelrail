const KEY = 'levelrail.banners.dismissed.v1'

export function parseDismissed(raw: string | null): string[] {
  if (!raw) return []
  try {
    const parsed: unknown = JSON.parse(raw)
    return Array.isArray(parsed)
      ? parsed.filter((v): v is string => typeof v === 'string')
      : []
  } catch {
    return []
  }
}

export function withDismissed(
  current: readonly string[],
  id: string,
): string[] {
  return current.includes(id) ? [...current] : [...current, id]
}

export function isBannerDismissed(id: string): boolean {
  try {
    return parseDismissed(window.localStorage.getItem(KEY)).includes(id)
  } catch {
    return false
  }
}

export function dismissBanner(id: string): void {
  try {
    const next = withDismissed(
      parseDismissed(window.localStorage.getItem(KEY)),
      id,
    )
    window.localStorage.setItem(KEY, JSON.stringify(next))
  } catch {
    // Storage blocked: the banner stays hidden for this view only.
  }
}
