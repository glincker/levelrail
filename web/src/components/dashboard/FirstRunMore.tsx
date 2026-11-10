import { useId, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDownGlassIcon } from '@thesvg/ui-react'
import { cn } from '@/lib/utils'

/** FirstRunMore folds secondary first-run content behind one quiet toggle so the page keeps a single primary action. */
export function FirstRunMore({
  children,
  label,
  closeLabel,
}: {
  children: ReactNode
  label?: string
  closeLabel?: string
}) {
  const { t } = useTranslation('firstRun')
  const [open, setOpen] = useState(false)
  const panelId = useId()
  return (
    <div className="flex flex-col items-start gap-4">
      <button
        type="button"
        aria-expanded={open}
        aria-controls={panelId}
        onClick={() => {
          setOpen((v) => !v)
        }}
        className="inline-flex min-h-8 items-center gap-1.5 rounded-md text-sm font-medium text-muted-foreground hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
      >
        {open ? (closeLabel ?? t('more.hide')) : (label ?? t('more.show'))}
        <ChevronDownGlassIcon
          aria-hidden="true"
          className={cn(
            'size-5 transition-transform motion-reduce:transition-none',
            open && 'rotate-180',
          )}
        />
      </button>
      <div id={panelId} hidden={!open} className="space-y-4">
        {children}
      </div>
    </div>
  )
}
