import { Dialog as DialogPrimitive } from '@base-ui/react/dialog'
import {
  DialogDescription,
  DialogOverlay,
  DialogPortal,
  DialogTitle,
} from '@/components/ui/dialog'
import { SHORTCUT_DOCS } from '@/lib/shortcuts'

export function ShortcutsDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
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
          <ul className="grid gap-1.5" aria-label="Shortcut list">
            {SHORTCUT_DOCS.map((s) => (
              <li
                key={s.description}
                className="flex items-center justify-between gap-4"
              >
                <span>{s.description}</span>
                <span className="flex items-center gap-1">
                  {s.keys.map((k, i) => (
                    <span key={k} className="flex items-center gap-1">
                      {i > 0 && (
                        <span className="text-xs text-muted-foreground">
                          then
                        </span>
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
        </DialogPrimitive.Popup>
      </DialogPortal>
    </DialogPrimitive.Root>
  )
}
