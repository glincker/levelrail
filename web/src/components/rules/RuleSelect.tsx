import type { ComponentProps } from 'react'
import { cn } from '@/lib/utils'

export interface RuleSelectOption {
  value: string
  label: string
}

/** RuleSelect is the one native select style shared by the rule builder and the firewall toolbars. */
export function RuleSelect({
  options,
  placeholder,
  className,
  ...props
}: Omit<ComponentProps<'select'>, 'children'> & {
  options: RuleSelectOption[]
  placeholder?: string
}) {
  return (
    <select
      {...props}
      className={cn(
        'h-8 min-w-0 rounded-lg border border-input bg-transparent px-2.5 text-sm text-foreground transition-colors outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive dark:bg-input/30',
        className,
      )}
    >
      {placeholder !== undefined ? (
        <option value="" disabled>
          {placeholder}
        </option>
      ) : null}
      {options.map((o) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </select>
  )
}
