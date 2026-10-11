import * as React from 'react'
import { cva, type VariantProps } from 'class-variance-authority'

import { cn } from '@/lib/utils'

const badgeVariants = cva(
  'inline-flex w-fit shrink-0 items-center justify-center gap-1 rounded-md border border-transparent px-2 py-0.5 text-xs font-medium whitespace-nowrap [&_svg]:pointer-events-none [&_svg]:size-3',
  {
    variants: {
      variant: {
        default: 'bg-secondary text-secondary-foreground',
        outline: 'border-border bg-transparent text-foreground',
        destructive:
          'border-tone-danger-border bg-tone-danger-soft text-tone-danger',
        muted: 'bg-muted text-muted-foreground',
        success:
          'border-tone-success-border bg-tone-success-soft text-tone-success',
        info: 'border-tone-info-border bg-tone-info-soft text-tone-info',
        warning:
          'border-tone-warning-border bg-tone-warning-soft text-tone-warning',
        accent:
          'border-tone-accent-border bg-tone-accent-soft text-tone-accent',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  },
)

function Badge({
  className,
  variant,
  ...props
}: React.ComponentProps<'span'> & VariantProps<typeof badgeVariants>) {
  return (
    <span
      data-slot="badge"
      className={cn(badgeVariants({ variant }), className)}
      {...props}
    />
  )
}

export { Badge, badgeVariants }
