import type { ComponentProps, ElementType, ReactNode } from 'react'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/lib/utils'

const displayVariants = cva(
  'font-[family-name:var(--glinr-font-display)] font-[var(--glinr-display-weight)] leading-tight tracking-tight text-balance text-foreground',
  {
    variants: {
      size: {
        lg: 'text-lg',
        '2xl': 'text-2xl',
        '3xl': 'text-2xl sm:text-3xl',
        '4xl': 'text-3xl sm:text-4xl',
      },
    },
    defaultVariants: { size: '2xl' },
  },
)

export interface DisplayProps
  extends
    Omit<ComponentProps<'h1'>, 'children'>,
    VariantProps<typeof displayVariants> {
  as?: 'h1' | 'h2' | 'h3' | 'p' | 'span'
  children: ReactNode
}

/** Display is the light, tight heading style shared by page heroes, banners and form sections. */
export function Display({
  as = 'h2',
  size,
  className,
  ...props
}: DisplayProps) {
  const Tag: ElementType = as
  return <Tag className={cn(displayVariants({ size }), className)} {...props} />
}

/** Eyebrow is the mono uppercase label with a leading accent dot above headings. */
export function Eyebrow({
  children,
  className,
  dot = true,
}: {
  children: ReactNode
  className?: string
  dot?: boolean
}) {
  return (
    <p
      className={cn(
        'flex items-center gap-2 font-[family-name:var(--glinr-font-eyebrow)] text-xs tracking-widest text-muted-foreground uppercase',
        className,
      )}
    >
      {dot ? (
        <span
          className="size-1.5 rounded-full bg-tone-accent-solid"
          aria-hidden="true"
        />
      ) : null}
      {children}
    </p>
  )
}

/** Lede is regular-weight muted supporting copy under a Display heading. */
export function Lede({ className, ...props }: ComponentProps<'p'>) {
  return (
    <p
      className={cn(
        'max-w-prose text-sm font-normal text-pretty text-muted-foreground',
        className,
      )}
      {...props}
    />
  )
}
