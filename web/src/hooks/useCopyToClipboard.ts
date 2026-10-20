import { useEffect, useRef, useState } from 'react'

// Copy-to-clipboard with a "Copied" state that resets itself after
// resetDelayMs, instead of staying "Copied" until something else in the
// component happens to re-render it. Several components (database
// public-access connection string, node join tokens, git source
// webhook secret) each hand-roll their own copied-state/button pair
// today; this is the shared version for new call sites, extracted here
// rather than at each of those, since touching all of them is a
// separate change nobody asked for.
export function useCopyToClipboard(resetDelayMs = 2000) {
  const [copied, setCopied] = useState(false)
  const timeoutRef = useRef<ReturnType<typeof setTimeout>>(undefined)

  useEffect(() => {
    return () => {
      clearTimeout(timeoutRef.current)
    }
  }, [])

  function copy(value: string) {
    if (!value) return
    void navigator.clipboard.writeText(value).then(() => {
      setCopied(true)
      clearTimeout(timeoutRef.current)
      timeoutRef.current = setTimeout(() => {
        setCopied(false)
      }, resetDelayMs)
    })
  }

  return { copied, copy }
}
