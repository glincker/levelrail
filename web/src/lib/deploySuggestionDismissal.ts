const PREFIX = 'levelrail.deploySuggestions.dismissed.v1.'

function key(appName: string): string {
  return PREFIX + appName
}

export function readDismissedSuggestions(appName: string): string[] {
  try {
    const raw = window.localStorage.getItem(key(appName))
    if (!raw) return []
    const parsed: unknown = JSON.parse(raw)
    return Array.isArray(parsed)
      ? parsed.filter((v): v is string => typeof v === 'string')
      : []
  } catch {
    return []
  }
}

export function dismissSuggestion(appName: string, id: string): string[] {
  const next = Array.from(new Set([...readDismissedSuggestions(appName), id]))
  try {
    window.localStorage.setItem(key(appName), JSON.stringify(next))
  } catch {
    // Storage blocked: the dismissal lasts for this view only.
  }
  return next
}

interface Dismissable {
  id: string
  params?: Record<string, string>
}

// A head-failure suggestion is about one commit, so its dismissal is too.
export function dismissalKey(s: Dismissable): string {
  return s.params?.sha ? `${s.id}:${s.params.sha}` : s.id
}

export function visibleSuggestions<T extends Dismissable>(
  suggestions: T[],
  dismissed: string[],
): T[] {
  return suggestions.filter((s) => !dismissed.includes(dismissalKey(s)))
}
