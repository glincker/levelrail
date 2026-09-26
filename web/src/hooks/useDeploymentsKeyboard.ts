import { useEffect, useRef } from 'react'
import { isTypingTarget } from '../lib/shortcuts'
import {
  deploymentKeyAction,
  type DeploymentKeyAction,
} from '../lib/deploymentsKeyboard'

function hasBlockingOverlay(): boolean {
  const dialogs = document.querySelectorAll('[role="dialog"], [role="menu"]')
  for (const el of dialogs) {
    if (!el.closest('[data-deployment-drawer]')) return true
  }
  return false
}

const INTERACTIVE = 'button, a, summary, [role="menuitem"], [role="tab"]'

/** Enter belongs to a focused control unless it is a deployment row itself. */
function ownsEnter(target: EventTarget | null): boolean {
  return (
    target instanceof HTMLElement &&
    target.closest(INTERACTIVE) !== null &&
    target.closest('[data-deployment-id]') === null
  )
}

export function useDeploymentsKeyboard(
  drawerOpen: boolean,
  onAction: (a: DeploymentKeyAction, e: KeyboardEvent) => void,
) {
  const ref = useRef({ drawerOpen, onAction })
  useEffect(() => {
    ref.current = { drawerOpen, onAction }
  })

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      const action = deploymentKeyAction({
        key: e.key,
        ctrl: e.ctrlKey,
        meta: e.metaKey,
        alt: e.altKey,
        typing: isTypingTarget(e.target),
        blockingOverlay: hasBlockingOverlay(),
        drawerOpen: ref.current.drawerOpen,
      })
      if (!action) return
      if (action.type === 'open' && ownsEnter(e.target)) return
      ref.current.onAction(action, e)
    }
    window.addEventListener('keydown', onKeyDown)
    return () => {
      window.removeEventListener('keydown', onKeyDown)
    }
  }, [])
}
