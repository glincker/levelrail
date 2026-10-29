import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { MessageCircleGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { cycleDockMode, toggleDock, useDockState } from './dockStore'

function useDockShortcut() {
  React.useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (!e.ctrlKey || e.metaKey || e.altKey) return
      const key = e.key.toLowerCase()
      if (key === 'j' && !e.shiftKey) {
        e.preventDefault()
        toggleDock()
      } else if (key === 'm' && e.shiftKey) {
        e.preventDefault()
        cycleDockMode()
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [])
}

export function DockToggle() {
  const { t } = useTranslation()
  const open = useDockState().view === 'open'
  useDockShortcut()
  return (
    <Button
      type="button"
      variant={open ? 'secondary' : 'ghost'}
      size="icon-sm"
      aria-label={t('dock.toggle')}
      aria-pressed={open}
      aria-keyshortcuts="Control+J Control+Shift+M"
      title={t('dock.toggleHint')}
      onClick={toggleDock}
    >
      <MessageCircleGlassIcon size="1.2em" />
    </Button>
  )
}
