import { useState } from 'react'

const key = (app: string) => `overview.dismissed.${app}`

function read(app: string): Set<string> {
  try {
    const raw = sessionStorage.getItem(key(app))
    return new Set(raw ? (JSON.parse(raw) as string[]) : [])
  } catch {
    return new Set()
  }
}

/** Suggestion ids the user dismissed, remembered for this browser session. */
export function useDismissed(app: string) {
  const [dismissed, setDismissed] = useState(() => read(app))
  const dismiss = (id: string) => {
    const next = new Set(dismissed).add(id)
    setDismissed(next)
    try {
      sessionStorage.setItem(key(app), JSON.stringify([...next]))
    } catch {
      // storage unavailable: dismissal lasts for this render tree only
    }
  }
  return { dismissed, dismiss }
}
