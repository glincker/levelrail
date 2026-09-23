import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'
import { HelpLink } from '@/components/HelpLink'

// Shared zero-item state for list pages and tables. Presentation only:
// the caller owns whichever create-resource flow the action opens.
export function EmptyState({
  icon,
  title,
  description,
  action,
  secondaryAction,
  hint,
  helpPath,
  helpLabel,
  className,
}: {
  /** A single Phosphor icon element, e.g. `<PackageIcon className="size-5" />`. */
  icon: ReactNode
  title: string
  description: string
  /** Primary action, typically a Button or a dialog/wizard trigger rendering one. Omit for a purely informational state. */
  action?: ReactNode
  /** Lower-emphasis action next to the primary one, e.g. "Start from a template". */
  secondaryAction?: ReactNode
  /** Short note pointing at an unmet prerequisite, e.g. why the primary action is a link to a different page. */
  hint?: ReactNode
  /** Docs path passed straight through to HelpLink; renders nothing if it resolves to neither a bundled page nor a configured docs URL. */
  helpPath?: string
  helpLabel?: string
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
      {hint ? (
        <p className="mx-auto max-w-sm text-xs text-muted-foreground">{hint}</p>
      ) : null}
      {action || secondaryAction ? (
        <div className="flex items-center gap-2">
          {action}
          {secondaryAction}
        </div>
      ) : null}
      {helpPath ? (
        <HelpLink
          path={helpPath}
          label={helpLabel ?? 'Learn more'}
          variant="inline"
        />
      ) : null}
    </div>
  )
}
