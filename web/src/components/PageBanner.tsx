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

  const iconArt = icon ? (
    <div
      aria-hidden="true"
      className={cn(
        'pointer-events-none absolute inset-y-0 left-0 hidden items-center [perspective:900px] sm:flex',
        compact ? 'w-40' : 'w-72',
        '[mask-image:linear-gradient(to_right,black_55%,transparent_100%)]',
      )}
    >
      <div
        className={cn(
          'absolute -left-10 size-72 rounded-full bg-[radial-gradient(closest-side,var(--iso-glow-warm),transparent)] opacity-80 transition-transform duration-300 ease-out motion-reduce:transition-none',
          '[transform:translate3d(calc(var(--px,0)*10px),calc(var(--py,0)*8px),0)]',
          compact && 'size-40',
        )}
      />
      <div
        className={cn(
          'relative ml-4 text-foreground/85 drop-shadow-[0_18px_28px_rgba(0,0,0,0.28)] transition-transform duration-300 ease-out motion-reduce:transition-none [&_svg]:size-full',
          '[transform:translate3d(calc(var(--px,0)*22px),calc(var(--py,0)*14px),0)_rotateY(calc(var(--px,0)*-10deg))_rotateX(calc(var(--py,0)*8deg))]',
          compact ? 'size-24' : 'size-44',
        )}
      >
        {icon}
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
        'relative isolate flex items-stretch overflow-hidden rounded-2xl border border-border bg-card',
        className,
      )}
    >
      {iconArt}
      <div
        className={cn(
          'relative z-10 flex min-w-0 flex-1 gap-x-3',
          compact
            ? 'min-h-[5.5rem] flex-wrap items-center px-4 py-4'
            : 'min-h-44 flex-col justify-center gap-y-3 px-6 py-7 md:max-w-xl md:flex-none',
          icon && (compact ? 'sm:pl-32' : 'sm:pl-60'),
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
