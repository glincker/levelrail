import { useEffect } from 'react'
import { isTypingTarget } from '@/lib/shortcuts'

interface Handlers {
  onOpenApp: () => void
  onCopyUrl: () => void
  onDeploy: () => void
}

/** O opens the app, C copies its URL, Shift+D opens the deploy sheet. */
export function useOverviewHotkeys({
  onOpenApp,
  onCopyUrl,
  onDeploy,
}: Handlers) {
  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.ctrlKey || e.metaKey || e.altKey) return
      if (isTypingTarget(e.target)) return
      if (document.querySelector('[role="dialog"]')) return
      if (e.key === 'o') onOpenApp()
      else if (e.key === 'c') onCopyUrl()
      else if (e.key === 'D') onDeploy()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [onOpenApp, onCopyUrl, onDeploy])
}
