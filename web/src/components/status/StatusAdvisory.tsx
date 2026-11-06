import { useId, useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDownGlassIcon } from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
import type { AttentionItem } from '@/lib/attention'

const STORAGE_KEY = 'levelrail.status.advisoryOpen'

function readOpen(): boolean {
  try {
    return window.localStorage.getItem(STORAGE_KEY) === '1'
  } catch {
    return false
  }
}

function writeOpen(open: boolean) {
  try {
    window.localStorage.setItem(STORAGE_KEY, open ? '1' : '0')
  } catch {
    // Storage can be blocked; the block just starts collapsed next time.
  }
}

// Recommendations are advice, not problems: one collapsed block keeps them
// reachable without interleaving them with real incidents.
export function StatusAdvisory({
  items,
  renderItem,
}: {
  items: AttentionItem[]
  renderItem: (item: AttentionItem) => ReactNode
}) {
  const { t } = useTranslation('attention')
  const [open, setOpen] = useState(readOpen)
  const panelId = useId()
  if (items.length === 0) return null
  return (
    <section aria-label={t('statusPage.advisory.region')}>
      <button
        type="button"
        aria-expanded={open}
        aria-controls={panelId}
        onClick={() => {
          writeOpen(!open)
          setOpen(!open)
        }}
        className="flex items-center gap-2 rounded-md px-1 py-1 text-sm font-medium text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
      >
        <ChevronDownGlassIcon
          aria-hidden="true"
          className={cn(
            'size-4 transition-transform motion-reduce:transition-none',
            open ? 'rotate-0' : '-rotate-90',
          )}
        />
        {t('statusPage.advisory.title', { count: items.length })}
      </button>
      <ul id={panelId} hidden={!open} className="mt-2 space-y-2">
        {items.map(renderItem)}
      </ul>
    </section>
  )
}
