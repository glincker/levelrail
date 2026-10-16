import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'
import {
  BrandIcon,
  hasMonoVariant,
  isBrandIconName,
  type BrandIconName,
} from './BrandIcon'
import { TemplateLogo } from './TemplateLogo'

// Neutral light tile so black or white-only marks stay legible in dark
// mode. Renders only the fallback when there is no logo id.
export function BrandLogoBadge({
  logoId,
  fallback,
  className = 'size-7',
}: {
  logoId: string | undefined
  fallback?: ReactNode
  className?: string
}) {
  if (!logoId) return <>{fallback ?? null}</>
  return (
    <span
      className={`inline-flex shrink-0 items-center justify-center rounded-md bg-white p-1 ring-1 ring-border ${className}`}
    >
      <TemplateLogo
        id={logoId}
        className="size-full"
        fallback={fallback ?? <span className="size-full" />}
      />
    </span>
  )
}

export type BrandTileSize = 'xs' | 'sm' | 'md' | 'lg' | 'xl'

const TILE_SIZE: Record<BrandTileSize, { box: string; mark: string }> = {
  xs: { box: 'size-5 rounded', mark: 'size-3.5' },
  sm: { box: 'size-8 rounded-md', mark: 'size-5' },
  md: { box: 'size-(--glinr-tile-size) rounded-lg', mark: 'size-6' },
  lg: { box: 'size-(--glinr-hero-tile-size) rounded-lg', mark: 'size-7' },
  xl: { box: 'size-12 rounded-xl', mark: 'size-7' },
}

export interface BrandTileProps {
  /** Brand mark key, or any string: unknown keys fall back to the initial. */
  name: string
  /** Provider name, used for the fallback initial and the accessible name. */
  label: string
  size?: BrandTileSize
  /** Tint the mark with currentColor (disconnected, disabled) when a mono mark exists. */
  muted?: boolean
  /** Treat the tile as a labelled image instead of decoration. */
  labelled?: boolean
  className?: string
  /** Custom mark (a lazily loaded logo) shown instead of the keyed icon or initial. */
  children?: ReactNode
}

/** BrandTile is the fixed square that holds a third-party brand mark (thesvg) with a mono and initial fallback. */
export function BrandTile({
  name,
  label,
  size = 'md',
  muted = false,
  labelled = false,
  className,
  children,
}: BrandTileProps) {
  const dims = TILE_SIZE[size]
  const known: BrandIconName | null = isBrandIconName(name) ? name : null
  const useMono = muted && known !== null && hasMonoVariant(known)
  return (
    <span
      role={labelled ? 'img' : undefined}
      aria-label={labelled ? label : undefined}
      aria-hidden={labelled ? undefined : true}
      data-slot="brand-tile"
      className={cn(
        'inline-flex shrink-0 items-center justify-center ring-1 ring-border',
        dims.box,
        // Brand colours are drawn for light surfaces, so the tile stays light in dark mode.
        useMono
          ? 'bg-muted text-muted-foreground'
          : 'bg-white text-neutral-900',
        muted && !useMono && 'opacity-60 grayscale',
        className,
      )}
    >
      {children ? (
        children
      ) : known ? (
        <BrandIcon name={known} mono={useMono} className={dims.mark} />
      ) : (
        <span className="font-mono text-sm font-medium uppercase">
          {label.slice(0, 1)}
        </span>
      )}
    </span>
  )
}
