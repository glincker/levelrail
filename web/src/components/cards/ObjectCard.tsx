import { Children, type ComponentProps, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Card } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

// auto-fill keeps empty tracks, so a lone card stays one column wide.
const GRID =
  'grid grid-cols-[repeat(auto-fill,minmax(min(100%,var(--card-min,18rem)),1fr))] gap-3'

const GRID_MIN = {
  sm: '[--card-min:15rem]',
  md: '[--card-min:18rem]',
  lg: '[--card-min:22rem]',
} as const

export function CardGrid({
  children,
  min = 'md',
  className,
  ...props
}: ComponentProps<'ul'> & { min?: keyof typeof GRID_MIN }) {
  return (
    <ul className={cn(GRID, GRID_MIN[min], className)} {...props}>
      {children}
    </ul>
  )
}

export function IconTile({
  children,
  className,
}: Readonly<{ children: ReactNode; className?: string }>) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        'flex size-10 shrink-0 items-center justify-center rounded-(--glinr-radius-inner) bg-muted text-muted-foreground ring-1 ring-foreground/5 ring-inset [&_svg]:size-5',
        className,
      )}
    >
      {children}
    </span>
  )
}

/** Quiet single line of facts; empty items are dropped so separators never double up. */
export function MetaRow({
  items,
  className,
}: Readonly<{ items: ReactNode[]; className?: string }>) {
  const shown = items.filter(
    (item) => item !== null && item !== undefined && item !== false,
  )
  if (shown.length === 0) return null
  return (
    <ul
      className={cn(
        'flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5 text-xs text-muted-foreground',
        className,
      )}
    >
      {Children.toArray(shown).map((item, i) => (
        <li key={i} className="flex min-w-0 items-center gap-x-1.5">
          {i > 0 ? (
            <span aria-hidden="true" className="text-muted-foreground/60">
              ·
            </span>
          ) : null}
          <span className="min-w-0 break-words">{item}</span>
        </li>
      ))}
    </ul>
  )
}

const HEADING = {
  h2: 'h2',
  h3: 'h3',
  h4: 'h4',
} as const

export type ObjectCardProps = Omit<ComponentProps<'div'>, 'title'> & {
  icon?: ReactNode
  title: ReactNode
  titleAs?: keyof typeof HEADING
  /** Top right, one status badge. */
  status?: ReactNode
  /** Small badges that sit after the title, such as Default or Recommended. */
  badges?: ReactNode
  meta?: ReactNode[]
  /** Footer actions, right aligned; pass the primary action last. */
  actions?: ReactNode
  /** Actions sit on the header row instead of a footer, for dense catalogue grids. */
  inlineActions?: boolean
  highlight?: boolean
}

/** One object (connection, method, step) as a card: header, optional body, footer actions. */
export function ObjectCard({
  icon,
  title,
  titleAs = 'h3',
  status,
  badges,
  meta,
  actions,
  inlineActions = false,
  highlight = false,
  children,
  className,
  ...props
}: ObjectCardProps) {
  const Heading = HEADING[titleAs]
  return (
    <Card
      data-slot="object-card"
      className={cn(
        'w-full min-w-0 transition-shadow duration-(--glinr-motion-fast) ease-(--glinr-motion-ease) hover:ring-foreground/20 has-focus-visible:ring-2 has-focus-visible:ring-ring motion-reduce:transition-none',
        highlight &&
          'ring-2 ring-tone-warning-border hover:ring-tone-warning-border',
        className,
      )}
      {...props}
    >
      <div className="flex items-start gap-3 px-(--card-spacing)">
        {icon ? <IconTile>{icon}</IconTile> : null}
        <div className="min-w-0 flex-1 space-y-1">
          <div className="flex items-start justify-between gap-2">
            <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
              <Heading className="min-w-0 truncate font-(family-name:--glinr-font-display) text-(length:--glinr-type-size-banner) leading-(--glinr-type-leading-banner) font-(weight:--glinr-type-heading-weight) text-foreground">
                {title}
              </Heading>
              {badges}
            </div>
            {status ? <div className="shrink-0">{status}</div> : null}
          </div>
          {meta ? <MetaRow items={meta} /> : null}
        </div>
        {inlineActions && actions ? (
          <div className="flex shrink-0 items-center gap-2 self-center">
            {actions}
          </div>
        ) : null}
      </div>
      {children ? (
        <div className="min-w-0 space-y-3 px-(--card-spacing) text-sm">
          {children}
        </div>
      ) : null}
      {!inlineActions && actions ? (
        <div className="mx-(--card-spacing) mt-auto flex flex-wrap items-center justify-end gap-2 border-t border-border pt-4">
          {actions}
        </div>
      ) : null}
    </Card>
  )
}

/** Loading placeholder with the same anatomy, so the grid does not jump on load. */
export function ObjectCardSkeleton({
  count = 3,
  footer = true,
  min = 'md',
}: Readonly<{
  count?: number
  footer?: boolean
  min?: keyof typeof GRID_MIN
}>) {
  const { t } = useTranslation('common')
  return (
    <div role="status" aria-label={t('cardSkeleton.loading')}>
      <CardGrid min={min} aria-hidden="true">
        {Array.from({ length: count }, (_, i) => (
          <li key={i} className="flex">
            <Card className="w-full">
              <div className="flex items-start gap-3 px-(--card-spacing)">
                <Skeleton
                  decorative
                  className="size-10 shrink-0 rounded-(--glinr-radius-inner)"
                />
                <div className="flex-1 space-y-2 pt-0.5">
                  <Skeleton decorative className="h-4 w-2/5" />
                  <Skeleton decorative className="h-3 w-3/5" />
                </div>
              </div>
              {footer ? (
                <div className="mx-(--card-spacing) flex justify-end border-t border-border pt-4">
                  <Skeleton decorative className="h-(--glinr-control-h) w-24" />
                </div>
              ) : null}
            </Card>
          </li>
        ))}
      </CardGrid>
    </div>
  )
}
