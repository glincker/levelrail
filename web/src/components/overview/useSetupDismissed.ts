import { useState } from 'react'

const storageKey = (app: string) => `overview.setup.dismissed.${app}`

/** Per-app, per-browser dismissal of the Recommended setup strip. */
export function useSetupDismissed(app: string) {
  const [dismissed, setDismissed] = useState(() => {
    try {
      return localStorage.getItem(storageKey(app)) === '1'
    } catch {
      return false
    }
  })
  const dismiss = () => {
    setDismissed(true)
    try {
      localStorage.setItem(storageKey(app), '1')
    } catch {
      // storage unavailable: dismissal lasts until reload
    }
  }
  return { dismissed, dismiss }
}
