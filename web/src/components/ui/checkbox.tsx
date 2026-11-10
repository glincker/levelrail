'use client'

import { Checkbox as CheckboxPrimitive } from '@base-ui/react/checkbox'
import { CheckIcon, MinusIcon } from '@thesvg/ui-react'

import { cn } from '@/lib/utils'

function Checkbox({ className, ...props }: CheckboxPrimitive.Root.Props) {
  return (
    <CheckboxPrimitive.Root
      data-slot="checkbox"
      className={cn(
        'peer relative flex size-4 shrink-0 items-center justify-center rounded-[4px] border border-(--glinr-border-control) bg-transparent text-white transition-colors outline-none after:absolute after:-inset-x-3 after:-inset-y-2 focus-visible:ring-2 focus-visible:ring-(--glinr-accent-solid) focus-visible:ring-offset-2 focus-visible:ring-offset-background motion-reduce:transition-none data-checked:border-(--glinr-accent-solid) data-checked:bg-(--glinr-accent-solid) data-indeterminate:border-(--glinr-accent-solid) data-indeterminate:bg-(--glinr-accent-solid) data-disabled:cursor-not-allowed data-disabled:opacity-50 group-has-disabled/field:opacity-50 aria-invalid:border-destructive aria-invalid:data-checked:bg-destructive aria-invalid:data-indeterminate:bg-destructive aria-invalid:inset-ring-1 aria-invalid:inset-ring-destructive/50',
        className,
      )}
      {...props}
    >
      <CheckboxPrimitive.Indicator
        data-slot="checkbox-indicator"
        className="grid place-content-center text-current transition-none"
      >
        {props.indeterminate ? (
          <MinusIcon size={12} />
        ) : (
          <CheckIcon size={12} />
        )}
      </CheckboxPrimitive.Indicator>
    </CheckboxPrimitive.Root>
  )
}

export { Checkbox }
