import { Dialog as DialogPrimitive } from '@base-ui/react/dialog'
import {
  DialogDescription,
  DialogOverlay,
  DialogPortal,
  DialogTitle,
} from '@/components/ui/dialog'
import { shortcutDocsFor, type ShortcutDoc } from '@/lib/shortcuts'
import { useExperimentalFeatures } from '@/hooks/useExperimental'
import { usePageActions } from '@/lib/pageActions'

function ShortcutGroup({
  label,
  items,
}: {
  label: string
  items: ShortcutDoc[]
}) {
  return (
    <div className="grid gap-1.5">
      <p className="text-xs font-medium text-muted-foreground">{label}</p>
      <ul className="grid gap-1.5" aria-label={`${label} shortcuts`}>
        {items.map((s) => (
          <li
            key={s.description}
            className="flex items-center justify-between gap-4"
          >
            <span>{s.description}</span>
            <span className="flex items-center gap-1">
              {s.keys.map((k, i) => (
                <span key={k} className="flex items-center gap-1">
                  {i > 0 && (
                    <span className="text-xs text-muted-foreground">then</span>
                  )}
                  <kbd className="rounded border border-border bg-muted px-1.5 py-0.5 text-xs">
                    {k}
                  </kbd>
                </span>
              ))}
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}

export function ShortcutsDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const experimental = useExperimentalFeatures()
  // Same registry the command palette reads, so "this page" never drifts
  // out of sync with the hints it already shows there.
  const pageShortcuts = usePageActions()
    .filter((a) => a.hint)
    .map((a) => ({ keys: a.hint as string[], description: a.label }))
  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPortal>
        <DialogOverlay />
        <DialogPrimitive.Popup
          aria-label="Keyboard shortcuts"
          className="fixed top-24 left-1/2 z-50 grid w-full max-w-md -translate-x-1/2 gap-3 rounded-xl bg-popover p-4 text-sm text-popover-foreground ring-1 ring-foreground/10 outline-none"
        >
          <DialogTitle>Keyboard shortcuts</DialogTitle>
          <DialogDescription>
            Shortcuts are off while typing in a field or when a dialog is open.
          </DialogDescription>
          <ShortcutGroup label="Global" items={shortcutDocsFor(experimental)} />
          {pageShortcuts.length > 0 && (
            <ShortcutGroup label="This page" items={pageShortcuts} />
          )}
        </DialogPrimitive.Popup>
      </DialogPortal>
    </DialogPrimitive.Root>
  )
}
