import type { MouseEvent } from 'react'
import { useTranslation } from 'react-i18next'

export const MAIN_CONTENT_ID = 'main-content'

// Focuses the main region directly: a bare hash jump does not move focus
// in every router history mode.
function focusMain(event: MouseEvent<HTMLAnchorElement>) {
  const main = document.getElementById(MAIN_CONTENT_ID)
  if (!main) return
  event.preventDefault()
  main.focus()
  main.scrollIntoView({ block: 'start' })
}

export function SkipLink() {
  const { t } = useTranslation()
  return (
    <a
      href={`#${MAIN_CONTENT_ID}`}
      onClick={focusMain}
      className="sr-only z-[60] rounded-md bg-background px-3 py-2 text-sm font-medium text-foreground shadow-md ring-2 ring-ring focus:not-sr-only focus:fixed focus:top-2 focus:left-2"
    >
      {t('shell.skipToContent')}
    </a>
  )
}
