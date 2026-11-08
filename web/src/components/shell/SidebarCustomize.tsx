import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SlidersHorizontalGlassIcon } from '@thesvg/ui-react'

// The dialog chunk loads on first open so it never weighs on the first paint.
const SidebarCustomizeDialog = lazy(() =>
  import('./SidebarCustomizeDialog').then((m) => ({
    default: m.SidebarCustomizeDialog,
  })),
)

export function SidebarCustomize() {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  return (
    <>
      <button
        type="button"
        aria-haspopup="dialog"
        onClick={() => setOpen(true)}
        className="flex h-8 w-full cursor-pointer items-center gap-2.5 rounded-md px-2.5 text-xs text-sidebar-foreground/75 outline-hidden transition-colors duration-150 hover:bg-state-accent/10 hover:text-sidebar-foreground focus-visible:ring-2 focus-visible:ring-sidebar-ring motion-reduce:transition-none pointer-coarse:h-10 group-data-[collapsible=icon]:hidden [&_svg]:size-4 [&_svg]:shrink-0"
      >
        <SlidersHorizontalGlassIcon aria-hidden="true" />
        {t('shell.customize.open')}
      </button>
      {open ? (
        <Suspense fallback={null}>
          <SidebarCustomizeDialog open={open} onOpenChange={setOpen} />
        </Suspense>
      ) : null}
    </>
  )
}
