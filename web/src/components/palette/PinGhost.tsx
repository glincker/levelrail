import { createPortal } from 'react-dom'

// Label that follows the pointer while a result is dragged toward the bubbles. Position comes from CSS variables.
export function PinGhost({
  ghostRef,
}: {
  ghostRef: React.RefObject<HTMLDivElement | null>
}) {
  if (typeof document === 'undefined') return null
  return createPortal(
    <div
      ref={ghostRef}
      hidden
      aria-hidden="true"
      className="palette-pill pointer-events-none fixed start-0 top-0 z-[100] translate-x-(--ghost-x,0px) translate-y-(--ghost-y,0px) rounded-full px-3 py-1 text-sm text-foreground"
    />,
    document.body,
  )
}
