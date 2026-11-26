// Maps an editor key event to an action. Run always wins over accept, so
// Cmd/Ctrl+Enter never inserts a suggestion.
export type EditorKeyAction =
  'run' | 'accept' | 'close' | 'next' | 'prev' | 'open' | 'none'

export interface KeyInput {
  key: string
  ctrlKey: boolean
  metaKey: boolean
  shiftKey: boolean
  altKey: boolean
  popupOpen: boolean
}

export function editorKeyAction(e: KeyInput): EditorKeyAction {
  if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
    return 'run'
  }
  if (e.ctrlKey && e.key === ' ') {
    return 'open'
  }
  if (!e.popupOpen) {
    return 'none'
  }
  const plain = !e.metaKey && !e.ctrlKey && !e.shiftKey && !e.altKey
  if (e.key === 'ArrowDown') return 'next'
  if (e.key === 'ArrowUp') return 'prev'
  if (e.key === 'Escape') return 'close'
  if (plain && (e.key === 'Enter' || e.key === 'Tab')) return 'accept'
  return 'none'
}
