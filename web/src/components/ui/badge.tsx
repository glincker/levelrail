import * as React from 'react'
import { cva, type VariantProps } from 'class-variance-authority'

import { cn } from '@/lib/utils'

// Not yet in the shadcn registry pull this repo has done (see
// TASKS.md's "shadcn/ui setup" entry), hand-written to match the same
// cva + data-slot conventions button.tsx and alert.tsx already
// establish, rather than reaching for a bespoke span in the component
// that needed it first (TokenTable's ability/revoked indicators).
const badgeVariants = cva(
  'inline-flex w-fit shrink-0 items-center justify-center gap-1 rounded-md border border-transparent px-2 py-0.5 text-xs font-medium whitespace-nowrap [&_svg]:pointer-events-none [&_svg]:size-3',
  {
    variants: {
      variant: {
        default: 'bg-secondary text-secondary-foreground',
        outline: 'border-border bg-transparent text-foreground',
        destructive:
          'bg-destructive/10 text-destructive dark:bg-destructive/20',
        muted: 'bg-muted text-muted-foreground',
        success:
          'bg-green-100 text-green-800 dark:bg-green-900/40 dark:text-green-300',
        warning:
          'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300',
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
