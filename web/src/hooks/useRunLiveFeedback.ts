import { useEffect, useRef } from 'react'
import { toast } from '@/components/ui/toast'

export type RunLiveState = 'running' | 'failed' | 'done' | 'idle'

const BADGE_ATTR = 'data-run-badge'
const BADGE_COLOR_VAR: Record<Exclude<RunLiveState, 'idle'>, string> = {
  running: '--glinr-accent-solid',
  failed: '--glinr-status-danger-solid',
  done: '--glinr-status-success-solid',
}

function setBadge(state: RunLiveState): void {
  document.querySelector(`link[${BADGE_ATTR}]`)?.remove()
  if (state === 'idle' || state === 'done') return
  const color = getComputedStyle(document.documentElement)
    .getPropertyValue(BADGE_COLOR_VAR[state])
    .trim()
  if (!color) return
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><circle cx="16" cy="16" r="12" fill="${color}"/></svg>`
  const link = document.createElement('link')
  link.rel = 'icon'
  link.type = 'image/svg+xml'
  link.href = `data:image/svg+xml,${encodeURIComponent(svg)}`
  link.setAttribute(BADGE_ATTR, 'true')
  document.head.appendChild(link)
}

/** Reflects a run in the tab title and icon, and toasts once when it ends. */
export function useRunLiveFeedback(
  state: RunLiveState,
  labels: {
    running: string
    failed: string
    doneToast: string
    failedToast: string
  },
): void {
  const { running, failed, doneToast, failedToast } = labels
  const prev = useRef<RunLiveState>(state)

  useEffect(() => {
    const original = document.title
    if (state === 'running') document.title = `${running} ${original}`
    if (state === 'failed') document.title = `${failed} ${original}`
    setBadge(state)
    return () => {
      document.title = original
      setBadge('idle')
    }
  }, [state, running, failed])

  useEffect(() => {
    if (prev.current === 'running' && state === 'done') {
      toast.add({ title: doneToast, type: 'success' })
    }
    if (prev.current === 'running' && state === 'failed') {
      toast.add({ title: failedToast, type: 'error' })
    }
    prev.current = state
  }, [state, doneToast, failedToast])
}
