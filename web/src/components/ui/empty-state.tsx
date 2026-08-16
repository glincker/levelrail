import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

// Shared zero-item state for list pages/tables: an icon in a muted
// circle, a one-line title, a slightly longer explainer, and an
// optional primary action. Extracted from what used to be near-
// identical inline markup duplicated across routes/apps/index.tsx,
// routes/databases/index.tsx, and routes/nodes/index.tsx (byte-for-byte
// the same wrapper classes, differing only in icon/copy/action), so a
// future resource list gets this for free instead of a fourth copy.
//
// Deliberately not a dialog, toast, or anything with its own state:
// this is presentation only, the caller still owns whichever
// create-resource flow the action button opens.
export function EmptyState({
  icon,
  title,
  description,
  action,
  className,
}: {
  /** A single Phosphor icon element, e.g. `<PackageIcon className="size-5" />`. */
  icon: ReactNode
  title: string
  description: string
  /** Typically a Button (or a dialog/wizard trigger rendering one). Omit for a purely informational empty state. */
  action?: ReactNode
  className?: string
}) {
  return (
    <div
      className={cn(
        'flex flex-col items-center gap-3 rounded-lg border border-dashed border-border bg-card/50 px-4 py-16 text-center',
        className,
      )}
    >
      <span
        className="flex size-10 items-center justify-center rounded-full bg-muted text-muted-foreground"
        aria-hidden="true"
      >
        {icon}
      </span>
      <div className="space-y-1">
        <p className="text-sm font-medium text-foreground">{title}</p>
        <p className="mx-auto max-w-sm text-sm text-muted-foreground">
          {description}
        </p>
      </div>
      {action}
    </div>
  )
}
