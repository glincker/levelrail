import * as React from 'react'
import { Input as InputPrimitive } from '@base-ui/react/input'

import { cn } from '@/lib/utils'
import { useFieldControl } from '@/components/ui/field-context'

function Input({ className, type, ...props }: React.ComponentProps<'input'>) {
  const wired = useFieldControl(props)
  return (
    <InputPrimitive
      type={type}
      data-slot="input"
      className={cn(
        'h-(--glinr-control-h) w-full min-w-0 rounded-(--glinr-radius-control) border border-input bg-transparent look-glass:border-(--glinr-border-control) look-glass:bg-(--glinr-surface-raised) look-glass:dark:border-white/15 look-glass:dark:bg-white/[0.05] px-2.5 py-1 text-base transition-colors outline-none file:inline-flex file:h-6 file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-(--glinr-accent-solid) focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none disabled:cursor-not-allowed disabled:bg-input/50 disabled:opacity-50 aria-invalid:border-destructive aria-invalid:inset-ring-1 aria-invalid:inset-ring-destructive/50 md:text-sm dark:bg-input/30 dark:disabled:bg-input/80',
        className,
      )}
      {...props}
      {...wired}
    />
  )
}

export { Input }
