import { cn } from '@/lib/utils'

/** DotBackdrop paints the brand dot grid and soft glow behind a banner; place it as the first child of a relative, overflow-hidden parent. */
export function DotBackdrop({ className }: { className?: string }) {
  return (
    <div
      aria-hidden="true"
      className={cn('pointer-events-none absolute inset-0', className)}
    >
      <div className="absolute inset-0 bg-[radial-gradient(circle_at_center,var(--color-foreground)_1px,transparent_1.5px)] [background-size:16px_16px] opacity-[0.12] [mask-image:linear-gradient(to_right,transparent,black_70%)]" />
      <div className="absolute -top-16 -right-10 size-56 rounded-full bg-primary/20 blur-3xl" />
      <div className="absolute -bottom-20 left-1/3 size-48 rounded-full bg-primary/10 blur-3xl" />
    </div>
  )
}
