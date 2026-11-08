import type { ComponentProps, ElementType, ReactNode } from 'react'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/lib/utils'

const displayVariants = cva(
  'font-(family-name:--glinr-font-display) tracking-tight text-balance text-foreground',
  {
    variants: {
      size: {
        banner:
          'text-(length:--glinr-type-size-banner) leading-(--glinr-type-leading-banner) font-(weight:--glinr-type-heading-weight)',
        section:
          'text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight)',
        page: 'text-(length:--glinr-type-size-page) leading-(--glinr-type-leading-page) font-(weight:--glinr-type-heading-weight)',
        greeting:
          'text-(length:--glinr-type-size-greeting) leading-(--glinr-type-leading-greeting) font-(weight:--glinr-type-greeting-weight)',
      },
    },
    defaultVariants: { size: 'page' },
  },
)

export interface DisplayProps
  extends
    Omit<ComponentProps<'h1'>, 'children'>,
    VariantProps<typeof displayVariants> {
  as?: 'h1' | 'h2' | 'h3' | 'p' | 'span'
  children: ReactNode
}

/** Display is the heading role (Geist, tight tracking) shared by page titles, banner and section titles, and the greeting. */
export function Display({
  as = 'h2',
  size,
  className,
  ...props
}: DisplayProps) {
  const Tag: ElementType = as
  return <Tag className={cn(displayVariants({ size }), className)} {...props} />
}

/** Eyebrow is the mono sentence-case label with a leading accent dot above headings. */
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
        'flex items-center gap-2 font-(family-name:--glinr-font-eyebrow) text-xs leading-4 text-muted-foreground',
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

/*
 * Label scale: FieldLabel 13px medium muted (sentence case, no tracking),
 * SectionTitle 14px semibold, Body text-sm. Eyebrow is a 12px sentence-case label.
 */
export function FieldLabel({
  as: Tag = 'p',
  className,
  ...props
}: ComponentProps<'p'> & { as?: 'p' | 'span' | 'dt' }) {
  const Component: ElementType = Tag
  return (
    <Component
      className={cn(
        'text-[13px] leading-5 font-medium text-muted-foreground',
        className,
      )}
      {...props}
    />
  )
}

export function SectionTitle({
  as: Tag = 'h3',
  className,
  ...props
}: ComponentProps<'h3'> & { as?: 'h2' | 'h3' | 'h4' | 'p' }) {
  const Component: ElementType = Tag
  return (
    <Component
      className={cn(
        'text-sm leading-5 font-semibold text-foreground',
        className,
      )}
      {...props}
    />
  )
}

export function Body({ className, ...props }: ComponentProps<'p'>) {
  return <p className={cn('text-sm text-foreground', className)} {...props} />
}
