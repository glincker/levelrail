import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { ChatCircleTextIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { toggleDock, useDockOpen } from './dockStore'

function useDockShortcut() {
  React.useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.ctrlKey && !e.metaKey && !e.altKey && e.key.toLowerCase() === 'j') {
        e.preventDefault()
        toggleDock()
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [])
}

export function DockToggle() {
  const { t } = useTranslation()
  const open = useDockOpen()
  useDockShortcut()
  return (
    <Button
      type="button"
      variant={open ? 'secondary' : 'ghost'}
      size="icon-sm"
      aria-label={t('dock.toggle')}
      aria-pressed={open}
      aria-keyshortcuts="Control+J"
      title={t('dock.toggleHint')}
      onClick={toggleDock}
    >
      <ChatCircleTextIcon />
    </Button>
  )
}
