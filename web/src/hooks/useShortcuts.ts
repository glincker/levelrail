import * as React from 'react'
import { useNavigate } from '@tanstack/react-router'
import {
  INITIAL_CHORD,
  findSearchField,
  isSearchField,
  isTypingTarget,
  stepChord,
  type ChordState,
} from '@/lib/shortcuts'

export function useShortcuts({ onHelp }: { onHelp: () => void }) {
  const navigate = useNavigate()
  const stateRef = React.useRef<ChordState>(INITIAL_CHORD)

  React.useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        stateRef.current = INITIAL_CHORD
        if (isSearchField(e.target) && e.target instanceof HTMLElement) {
          e.target.blur()
        }
        return
      }
      const result = stepChord(
        stateRef.current,
        {
          key: e.key,
          ctrl: e.ctrlKey,
          meta: e.metaKey,
          alt: e.altKey,
          typing: isTypingTarget(e.target),
          dialogOpen: document.querySelector('[role="dialog"]') !== null,
        },
        e.timeStamp,
      )
      stateRef.current = result.state
      const action = result.action
      if (!action) return
      if (action.type === 'go') {
        e.preventDefault()
        void navigate({ to: action.to })
      } else if (action.type === 'help') {
        e.preventDefault()
        onHelp()
      } else {
        const field = findSearchField()
        if (field) {
          e.preventDefault()
          field.focus()
        }
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [navigate, onHelp])
}
