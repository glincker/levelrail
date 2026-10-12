import type { ComponentProps } from 'react'
import { Skeleton as GlinSkeleton } from '@glinui/ui'
import { cn } from '@/lib/utils'

export type SkeletonVariant =
  'glinr' | 'solid' | 'plain' | 'soft' | 'outline' | 'ghost'

// @glinui/ui 0.1.1 ships default|glass|liquid|matte|outline|ghost; the docs'
// glinr|solid|plain|soft names are not published yet, so map onto what exists.
// Not matte: its dark gradient (rgb 40,42,48) tints blue and adds a border.
const PUBLISHED = {
  glinr: 'default',
  solid: 'default',
  plain: 'default',
  soft: 'ghost',
  outline: 'outline',
  ghost: 'ghost',
} as const

type SkeletonProps = ComponentProps<'div'> & {
  variant?: SkeletonVariant
  decorative?: boolean
}

function Skeleton({
  className,
  variant = 'glinr',
  decorative = false,
  ...props
}: SkeletonProps) {
  return (
    <GlinSkeleton
      data-slot="skeleton"
      variant={PUBLISHED[variant]}
      size={null}
      decorative={decorative}
      className={cn('rounded-md', className)}
      {...props}
    />
  )
}

export { Skeleton }
