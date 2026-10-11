import * as React from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useExperimentalFeatures } from './useExperimental'
import { useRouteAvailable } from '@/lib/routeAvailability'
import {
  INITIAL_CHORD,
  LONG_PRESS_MS,
  findSearchField,
  isDialogOpen,
  isSearchField,
  isShortcutInputSuppressed,
  isTypingTarget,
  stepChord,
  type ChordState,
  type KeyInput,
} from '@/lib/shortcuts'

function keyInputFrom(e: KeyboardEvent): KeyInput {
  return {
    key: e.key,
    ctrl: e.ctrlKey,
    meta: e.metaKey,
    alt: e.altKey,
    typing: isTypingTarget(e.target),
    dialogOpen: isDialogOpen(),
  }
}

export function useShortcuts({
  onOpenStageOverlay,
}: {
  onOpenStageOverlay: () => void
}) {
  const navigate = useNavigate()
  const experimental = useExperimentalFeatures()
  const routeAvailable = useRouteAvailable()
  const stateRef = React.useRef<ChordState>(INITIAL_CHORD)
  const longPressTimerRef = React.useRef<number | null>(null)

  React.useEffect(() => {
    function clearLongPress() {
      if (longPressTimerRef.current !== null) {
        window.clearTimeout(longPressTimerRef.current)
        longPressTimerRef.current = null
      }
    }
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        stateRef.current = INITIAL_CHORD
        clearLongPress()
        if (isSearchField(e.target) && e.target instanceof HTMLElement) {
          e.target.blur()
        }
        return
      }
      const wasChordPending = stateRef.current.pending
      const input = keyInputFrom(e)
      const result = stepChord(
        stateRef.current,
        input,
        e.timeStamp,
        experimental,
        routeAvailable,
      )
      stateRef.current = result.state
      const action = result.action
      if (action) {
        if (action.type === 'go') {
          e.preventDefault()
          void navigate({ to: action.to })
        } else {
          const field = findSearchField()
          if (field) {
            e.preventDefault()
            field.focus()
          }
        }
        return
      }
      // A bare hold of "l" summons the stage overlay. The g-then-l chord
      // (Load balancers) is excluded via wasChordPending so a quick g, l
      // tap never also starts this timer.
      if (
        !wasChordPending &&
        !e.repeat &&
        e.key.toLowerCase() === 'l' &&
        !isShortcutInputSuppressed(input)
      ) {
        clearLongPress()
        longPressTimerRef.current = window.setTimeout(() => {
          longPressTimerRef.current = null
          onOpenStageOverlay()
        }, LONG_PRESS_MS)
      }
    }
    function onKeyUp(e: KeyboardEvent) {
      if (e.key.toLowerCase() === 'l') clearLongPress()
    }
    window.addEventListener('keydown', onKeyDown)
    window.addEventListener('keyup', onKeyUp)
    window.addEventListener('blur', clearLongPress)
    return () => {
      window.removeEventListener('keydown', onKeyDown)
      window.removeEventListener('keyup', onKeyUp)
      window.removeEventListener('blur', clearLongPress)
      clearLongPress()
    }
  }, [navigate, onOpenStageOverlay, experimental, routeAvailable])
}
