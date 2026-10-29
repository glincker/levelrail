import { useEffect, useRef, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { useRegisterPageTitle } from './shell/pageTitleStore'

export type PageBannerSize = 'hero' | 'compact'

const EDGE_FADE =
  '[mask-image:radial-gradient(ellipse_at_center,black_55%,transparent_100%)]'
const GLOW =
  'bg-[radial-gradient(60%_55%_at_55%_0%,var(--iso-glow-warm),transparent),radial-gradient(60%_55%_at_45%_100%,var(--iso-glow-cool),transparent)]'

// Pointer parallax: writes --px/--py (-1 to 1) on the header, which the
// layers read through CSS transforms. Skipped for reduced motion.
function useParallax() {
  const ref = useRef<HTMLElement>(null)
  useEffect(() => {
    const el = ref.current
    if (!el || typeof window.matchMedia !== 'function') return
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return
    let frame = 0
    const set = (x: number, y: number) => {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(() => {
        el.style.setProperty('--px', x.toFixed(3))
        el.style.setProperty('--py', y.toFixed(3))
      })
    }
    const move = (e: PointerEvent) => {
      const r = el.getBoundingClientRect()
      set(
        ((e.clientX - r.left) / r.width - 0.5) * 2,
        ((e.clientY - r.top) / r.height - 0.5) * 2,
      )
    }
    const leave = () => {
      set(0, 0)
    }
    el.addEventListener('pointermove', move)
    el.addEventListener('pointerleave', leave)
    return () => {
      cancelAnimationFrame(frame)
      el.removeEventListener('pointermove', move)
      el.removeEventListener('pointerleave', leave)
    }
  }, [])
  return ref
}

// Keys carry their namespace prefix (e.g. "functions:banner.title").
// The banner is the page header: it registers the title and renders the h1.
export function PageBanner({
  icon,
  titleKey,
  titleText,
  descriptionKey,
  descriptionText,
  action,
  illustration,
  size = 'hero',
  className,
}: {
  icon?: ReactNode
  titleKey: string
  titleText?: string
  descriptionKey: string
  descriptionText?: string
  action?: ReactNode
  illustration?: ReactNode
  size?: PageBannerSize
  className?: string
}) {
  const { t } = useTranslation()
  const title = titleText ?? t(titleKey, { defaultValue: titleKey })
  const description =
    descriptionText ?? t(descriptionKey, { defaultValue: descriptionKey })
  useRegisterPageTitle(title)
  const compact = size === 'compact'
  const headerRef = useParallax()

  // Three stacked copies fake depth: two dim, offset copies behind the sharp front one.
  const layer = (depth: string, cls: string) => (
    <div
      className={cn(
        'absolute inset-0 transition-transform duration-300 ease-out motion-reduce:transition-none [&_svg]:size-full',
        depth,
        cls,
      )}
    >
      {icon}
    </div>
  )
  const iconArt = icon ? (
    <div
      aria-hidden="true"
      className={cn(
        'pointer-events-none absolute left-0 hidden [perspective:700px] sm:block',
        compact ? '-bottom-4 -left-2 size-32' : '-bottom-10 -left-4 size-56',
        '[mask-image:linear-gradient(to_right,black_40%,transparent_100%)]',
      )}
    >
      <div
        className={cn(
          'absolute inset-0 text-foreground/80 [transform-style:preserve-3d]',
          '[transform:translate3d(calc(var(--px,0)*14px),calc(var(--py,0)*10px),0)_rotateY(calc(-26deg+var(--px,0)*-8deg))_rotateX(calc(12deg+var(--py,0)*6deg))_rotateZ(-10deg)]',
          'transition-transform duration-300 ease-out motion-reduce:transition-none',
        )}
      >
        {layer(
          '[transform:translate3d(18px,22px,-40px)]',
          'text-foreground/20 blur-[2px]',
        )}
        {layer('[transform:translate3d(9px,11px,-20px)]', 'text-foreground/35')}
        {layer('', 'opacity-55 drop-shadow-[0_14px_22px_rgba(0,0,0,0.35)]')}
      </div>
    </div>
  ) : null

  const art = illustration ? (
    <div
      aria-hidden="true"
      className={cn(
        'relative hidden md:block',
        compact ? 'w-56 shrink-0 self-stretch' : 'min-h-52 flex-1',
      )}
    >
      <div className={cn('absolute inset-0', GLOW, EDGE_FADE)} />
      <div
        className={cn(
          'absolute inset-0 flex items-center justify-center',
          compact ? 'py-1' : 'p-1',
          EDGE_FADE,
        )}
      >
        {illustration}
      </div>
    </div>
  ) : null

  return (
    <header
      ref={headerRef}
      data-testid={compact ? 'page-banner-compact' : 'page-banner-full'}
      className={cn(
        'relative isolate flex items-stretch overflow-hidden rounded-2xl border border-border bg-card [&_[data-slot=help-link]]:absolute [&_[data-slot=help-link]]:right-3 [&_[data-slot=help-link]]:bottom-3 [&_[data-slot=help-link]]:z-20',
        className,
      )}
    >
      {iconArt}
      <div
        className={cn(
          'z-10 flex min-w-0 flex-1 gap-x-3',
          compact
            ? 'min-h-[5.5rem] flex-wrap items-center px-4 py-4'
            : 'min-h-44 flex-col justify-center gap-y-3 px-6 py-7 md:max-w-xl md:flex-none',
          icon && (compact ? 'sm:pl-24' : 'sm:pl-28'),
        )}
      >
        <div
          className={cn(
            'flex min-w-0 items-center gap-3',
            compact && 'flex-1 basis-64',
          )}
        >
          <div className="min-w-0">
            <h1
              className={cn(
                'bg-gradient-to-b from-foreground to-foreground/65 bg-clip-text font-bold tracking-tighter text-transparent drop-shadow-[0_6px_14px_rgba(0,0,0,0.16)]',
                compact ? 'text-xl' : 'text-3xl md:text-4xl',
              )}
            >
              {title}
            </h1>
            {compact ? (
              <p className="truncate text-sm text-muted-foreground">
                {description}
              </p>
            ) : null}
          </div>
        </div>
        {compact ? null : (
          <p className="line-clamp-2 max-w-prose text-sm text-muted-foreground md:text-base">
            {description}
          </p>
        )}
        {action ? (
          <div
            className={cn(
              'flex flex-wrap items-center gap-2',
              compact && 'ml-auto',
            )}
          >
            {action}
          </div>
        ) : null}
      </div>
      {art}
    </header>
  )
}
