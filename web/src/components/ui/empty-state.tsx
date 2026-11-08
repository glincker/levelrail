import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'
import { TONE, type ToneClasses } from '@/components/kit/tone'
import { HelpLink } from '@/components/HelpLink'
import {
  Illustration,
  type IllustrationName,
} from '@/components/kit/illustrations'

export type EmptyStateKind = 'first-run' | 'no-results' | 'error' | 'permission'

const KIND_TONE: Record<EmptyStateKind, ToneClasses> = {
  'first-run': TONE.accent,
  'no-results': TONE.neutral,
  error: TONE.danger,
  permission: TONE.warning,
}

// Shared zero-item state for list pages and tables. Presentation only:
// the caller owns whichever create-resource flow the action opens.
export function EmptyState({
  icon,
  illustration,
  title,
  description,
  children,
  action,
  secondaryAction,
  hint,
  helpPath,
  helpLabel,
  compact = false,
  kind,
  className,
}: {
  /** Left aligned, action beside the copy, tone by situation. Omit for the centred default. */
  kind?: EmptyStateKind
  /** A single `@thesvg/ui-react` icon element, e.g. `<PackageGlassIcon className="size-6" />`. */
  icon: ReactNode
  /** Decorative SVG shown in place of the icon on higher-emphasis, top-of-funnel zero states (e.g. the first-ever app). */
  illustration?: IllustrationName
  title: string
  description: string
  /** Extra content between the description and the action row, e.g. a short code sample. */
  children?: ReactNode
  /** Primary action, typically a Button or a dialog/wizard trigger rendering one. Omit for a purely informational state. */
  action?: ReactNode
  /** Lower-emphasis action next to the primary one, e.g. "Start from a template". */
  secondaryAction?: ReactNode
  /** Short note pointing at an unmet prerequisite, e.g. why the primary action is a link to a different page. */
  hint?: ReactNode
  /** Docs path passed straight through to HelpLink; renders nothing if it resolves to neither a bundled page nor a configured docs URL. */
  helpPath?: string
  helpLabel?: string
  /** Inline one-row state (icon, title, muted line), no dashed outline: for empty lists inside a page section. */
  compact?: boolean
  className?: string
}) {
  if (kind) {
    const tone = KIND_TONE[kind]
    return (
      <div
        className={cn(
          'flex flex-wrap items-center gap-4 rounded-glinr-group border px-4 py-4 text-left',
          tone.border,
          tone.soft,
          className,
        )}
      >
        <span
          className={cn(
            'flex size-9 shrink-0 items-center justify-center rounded-full bg-card',
            tone.text,
          )}
          aria-hidden="true"
        >
          {icon}
        </span>
        <div className="min-w-0 flex-1 space-y-0.5">
          <p className="text-sm font-medium text-foreground">{title}</p>
          <p className="text-sm text-muted-foreground">{description}</p>
          {hint ? (
            <p className="text-xs text-muted-foreground">{hint}</p>
          ) : null}
        </div>
        {action || secondaryAction ? (
          <div className="flex shrink-0 items-center gap-2">
            {secondaryAction}
            {action}
          </div>
        ) : null}
      </div>
    )
  }
  if (compact) {
    return (
      <div
        className={cn(
          'flex items-center gap-3 rounded-lg bg-muted/40 px-4 py-4',
          className,
        )}
      >
        <span
          className="flex size-8 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground"
          aria-hidden="true"
        >
          {icon}
        </span>
        <div className="min-w-0 space-y-0.5">
          <p className="text-sm font-medium text-foreground">{title}</p>
          <p className="text-xs text-muted-foreground">{description}</p>
        </div>
        {action ? <div className="ml-auto shrink-0">{action}</div> : null}
      </div>
    )
  }
  return (
    <div
      className={cn(
        'flex flex-col items-center gap-2 rounded-lg border border-border bg-card px-4 py-5 text-center',
        className,
      )}
    >
      {illustration ? (
        <span
          className="text-muted-foreground/70 [&_svg]:max-h-12 [&_svg]:w-auto"
          aria-hidden="true"
        >
          <Illustration name={illustration} />
        </span>
      ) : (
        <span
          className="flex size-8 items-center justify-center rounded-full bg-muted text-muted-foreground"
          aria-hidden="true"
        >
          {icon}
        </span>
      )}
      <div className="space-y-1">
        <p className="text-sm font-medium text-foreground">{title}</p>
        <p className="mx-auto max-w-sm text-sm text-muted-foreground">
          {description}
        </p>
      </div>
      {hint ? (
        <p className="mx-auto max-w-sm text-xs text-muted-foreground">{hint}</p>
      ) : null}
      {children}
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
