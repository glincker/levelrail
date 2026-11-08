import type { ComponentProps, ReactNode } from 'react'
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

// Stacks skeleton and content in one grid cell and crossfades opacity only, so the layout never jumps.
function SkeletonSwap({
  loading,
  skeleton,
  children,
  className,
}: {
  loading: boolean
  skeleton: ReactNode
  children: ReactNode
  className?: string
}) {
  return (
    <div
      aria-busy={loading}
      className={cn('grid [&>*]:col-start-1 [&>*]:row-start-1', className)}
    >
      <div
        aria-hidden="true"
        className={cn(
          'transition-opacity duration-200 motion-reduce:transition-none',
          loading ? 'opacity-100' : 'pointer-events-none opacity-0',
        )}
      >
        {skeleton}
      </div>
      <div
        className={cn(
          'transition-opacity duration-200 motion-reduce:transition-none',
          loading ? 'pointer-events-none opacity-0' : 'opacity-100',
        )}
      >
        {children}
      </div>
    </div>
  )
}

export { Skeleton, SkeletonSwap }
