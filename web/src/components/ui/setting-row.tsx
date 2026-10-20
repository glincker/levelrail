import { useId, type ReactNode } from 'react'
import { cn } from '@/lib/utils'

export interface SettingRowProps {
  title: ReactNode
  description?: ReactNode
  /** Small status element beside the title. */
  badge?: ReactNode
  className?: string
  /** The control: input, switch, button group or read-only value. */
  children: ReactNode
}

/** SettingRows stacks SettingRow children with a divider between each. */
export function SettingRows({
  className,
  children,
}: {
  className?: string
  children: ReactNode
}) {
  return (
    <div className={cn('divide-y divide-border', className)}>{children}</div>
  )
}

/** SettingRow puts title and helper text left and the control right from md up, one column below. */
export function SettingRow({
  title,
  description,
  badge,
  className,
  children,
}: SettingRowProps) {
  const titleId = useId()
  const descId = useId()
  return (
    <div
      role="group"
      aria-labelledby={titleId}
      aria-describedby={description ? descId : undefined}
      className={cn(
        'grid gap-x-8 gap-y-2 py-4 first:pt-0 last:pb-0 md:grid-cols-[minmax(0,14rem)_minmax(0,1fr)] lg:grid-cols-[minmax(0,18rem)_minmax(0,1fr)]',
        className,
      )}
    >
      <div className="min-w-0 space-y-0.5">
        <div className="flex flex-wrap items-center gap-2 text-sm font-medium text-foreground">
          <span id={titleId}>{title}</span>
          {badge}
        </div>
        {description ? (
          <p id={descId} className="text-xs text-muted-foreground">
            {description}
          </p>
        ) : null}
      </div>
      <div className="min-w-0 md:max-w-xl">{children}</div>
    </div>
  )
}
