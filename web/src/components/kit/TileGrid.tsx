import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { prefersReducedMotion } from '@/lib/reducedMotion'
import { cn } from '@/lib/utils'
import { SkeletonLine } from './Skeleton'

export type TileSize = 'S' | 'M' | 'L'

// Spans on the 12-column grid: S is 3 of 12, M is 6, L is 12. Below xl the
// grid has 6 columns (md) or 1 (base), so spans are clamped per breakpoint.
const TILE_SPAN: Record<TileSize, string> = {
  S: 'md:col-span-3 xl:col-span-3',
  M: 'md:col-span-6 xl:col-span-6',
  L: 'md:col-span-6 xl:col-span-12',
}

const TILE_ROWS: Record<1 | 2 | 3, string> = {
  1: 'row-span-1',
  2: 'row-span-2',
  3: 'row-span-3',
}

// Stagger steps are static classes (no inline styles); capped at 8 tiles.
const STAGGER =
  '[&>*:nth-child(2)]:[animation-delay:30ms] [&>*:nth-child(3)]:[animation-delay:60ms] [&>*:nth-child(4)]:[animation-delay:90ms] [&>*:nth-child(5)]:[animation-delay:120ms] [&>*:nth-child(6)]:[animation-delay:150ms] [&>*:nth-child(7)]:[animation-delay:180ms] [&>*:nth-child(n+8)]:[animation-delay:210ms]'

export interface TileGridProps {
  children: ReactNode
  className?: string
}

/** TileGrid is a dense 12-column grid: 1 column on mobile, 6 from md, 12 from xl. */
export function TileGrid({ children, className }: TileGridProps) {
  return (
    <div
      data-testid="tile-grid"
      className={cn(
        'grid grid-flow-dense grid-cols-1 gap-4 md:grid-cols-6 xl:grid-cols-12',
        STAGGER,
        className,
      )}
    >
      {children}
    </div>
  )
}

export interface TileProps {
  title: string
  size?: TileSize
  rows?: 1 | 2 | 3
  icon?: ReactNode
  /** Right side of the header. */
  action?: ReactNode
  /** Optional link or button row under the body. */
  footer?: ReactNode
  loading?: boolean
  error?: boolean
  /** Message shown in the error state. */
  errorLabel?: string
  onRetry?: () => void
  /** With hideWhenEmpty, an empty tile renders nothing and the grid reflows. */
  empty?: boolean
  hideWhenEmpty?: boolean
  className?: string
  children?: ReactNode
}

/** Tile is one card on a TileGrid with header, body, footer, loading and error states. */
export function Tile({
  title,
  size = 'M',
  rows = 1,
  icon,
  action,
  footer,
  loading,
  error,
  errorLabel,
  onRetry,
  empty,
  hideWhenEmpty,
  className,
  children,
}: TileProps) {
  const { t } = useTranslation('common')
  if (hideWhenEmpty && empty) return null
  const animate = !prefersReducedMotion()
  return (
    <section
      aria-label={title}
      aria-busy={loading || undefined}
      data-testid="tile"
      data-size={size}
      className={cn(
        'flex min-w-0 flex-col gap-3 rounded-xl bg-card p-(--glinr-card-pad) text-sm text-card-foreground shadow-card ring-1 ring-foreground/10 motion-safe:transition-[transform,box-shadow] motion-safe:hover:-translate-y-px motion-safe:hover:shadow-raised',
        animate && 'kit-tile-enter',
        TILE_SPAN[size],
        TILE_ROWS[rows],
        className,
      )}
    >
      <header className="flex items-center gap-2">
        {icon ? (
          <span className="text-state-accent" aria-hidden="true">
            {icon}
          </span>
        ) : null}
        <h3 className="min-w-0 flex-1 truncate text-sm font-medium">{title}</h3>
        {action ? <div className="shrink-0">{action}</div> : null}
      </header>
      <div className="min-h-0 flex-1">
        {loading ? (
          <div data-testid="tile-skeleton" className="flex flex-col gap-2">
            <SkeletonLine width="60%" />
            <SkeletonLine className="h-6" />
            <SkeletonLine width="80%" />
          </div>
        ) : error ? (
          <div
            role="alert"
            className="flex flex-wrap items-center gap-2 text-tone-danger-fg"
          >
            <TriangleAlertGlassIcon size="1.4em" aria-hidden="true" />
            {errorLabel ? <span>{errorLabel}</span> : null}
            {onRetry ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={onRetry}
              >
                {t('kit.retry')}
              </Button>
            ) : null}
          </div>
        ) : (
          children
        )}
      </div>
      {footer && !loading && !error ? (
        <footer className="text-xs">{footer}</footer>
      ) : null}
    </section>
  )
}
