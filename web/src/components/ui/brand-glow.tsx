import { cn } from '@/lib/utils'

/** BrandGlow paints the login panel's grainy brand wash; place it as the first child of a relative, overflow-hidden, dark parent. */
export function BrandGlow({ className }: { className?: string }) {
  return (
    <div
      aria-hidden="true"
      className={cn('pointer-events-none absolute inset-0', className)}
    >
      <div className="brand-glow absolute inset-x-0 top-0 h-full" />
      <div className="brand-glow-grain absolute inset-x-0 top-0 h-full" />
    </div>
  )
}
