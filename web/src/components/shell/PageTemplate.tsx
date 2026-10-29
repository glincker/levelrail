import { useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { HelpLink } from '../HelpLink'
import { Display, Eyebrow, Lede } from '@/components/ui/typography'
import { cn } from '@/lib/utils'
import { useRegisterPageTitle } from './pageTitleStore'

const DESCRIPTION_LINE_PX = 24

export type PageTemplateWidth = 'default' | 'wide' | 'full'

const WIDTH: Record<PageTemplateWidth, string> = {
  default: 'max-w-(--glinr-content-max)',
  wide: 'max-w-(--glinr-content-max-wide)',
  full: 'max-w-none',
}

export interface PageTemplateProps {
  title: ReactNode
  description?: ReactNode
  /** Small mono label above the title. */
  eyebrow?: ReactNode
  /** A BrandTile, or any hero tile. Wins over `icon`. */
  logo?: ReactNode
  /** Phosphor icon rendered in a neutral hero tile. */
  icon?: ReactNode
  /** Status badge or pill beside the title. */
  status?: ReactNode
  /** Primary action(s), right-aligned from sm up. */
  actions?: ReactNode
  helpPath?: string
  helpLabel?: string
  breadcrumb?: ReactNode
  /** Tab list or link nav rendered under the hero. */
  tabs?: ReactNode
  /** Page-level banners, rendered between the hero and the content. */
  banners?: ReactNode
  width?: PageTemplateWidth
  /** Right rail, 20rem and sticky from xl (1280px of the content area), after the main content below it. */
  aside?: ReactNode
  /** Accessible name of the aside landmark. */
  asideLabel?: string
  /** `page` (20px) by default; `greeting` (28px) is for the dashboard greeting only. */
  titleSize?: 'page' | 'greeting'
  /** A PageBanner that replaces the plain header, e.g. an empty list. */
  banner?: ReactNode
  className?: string
  children?: ReactNode
}

/** PageTemplate is the shared page shell: hero with tile, title, status, actions, help, tabs, then content. */
export function PageTemplate({
  title,
  description,
  eyebrow,
  logo,
  icon,
  status,
  actions,
  helpPath,
  helpLabel,
  breadcrumb,
  tabs,
  banners,
  width = 'default',
  aside,
  asideLabel,
  titleSize = 'page',
  banner,
  className,
  children,
}: PageTemplateProps) {
  useRegisterPageTitle(title)
  const descRef = useRef<HTMLDivElement>(null)
  const [wrapped, setWrapped] = useState(false)
  useLayoutEffect(() => {
    const el = descRef.current
    if (!el) return
    const measure = () => setWrapped(el.offsetHeight > DESCRIPTION_LINE_PX)
    measure()
    if (typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [description])

  const tile =
    logo ??
    (icon ? (
      <span
        aria-hidden="true"
        className="flex size-(--glinr-hero-tile-size) shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground ring-1 ring-border [&_svg]:size-6"
      >
        {icon}
      </span>
    ) : null)

  return (
    <div
      data-page-width={width}
      className={cn(
        '@container/page mx-auto flex w-full min-w-0 flex-col gap-(--glinr-page-gap)',
        WIDTH[width],
        className,
      )}
    >
      {banner ?? (
        <header className="space-y-3">
          {breadcrumb ? <div>{breadcrumb}</div> : null}
          <div
            data-slot="page-hero"
            data-wrapped={wrapped}
            className={cn(
              'flex flex-wrap gap-3',
              wrapped ? 'items-start' : 'items-center',
            )}
          >
            {tile ? (
              <div
                data-slot="page-hero-tile"
                className={cn('shrink-0', wrapped && eyebrow && 'mt-5')}
              >
                {tile}
              </div>
            ) : null}
            <div className="min-w-0 flex-1 basis-64 space-y-1">
              {eyebrow ? <Eyebrow>{eyebrow}</Eyebrow> : null}
              <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                <Display
                  as="h1"
                  size={titleSize}
                  className="min-w-0 break-words"
                >
                  {title}
                </Display>
                {status}
                {helpPath && helpLabel ? (
                  <HelpLink path={helpPath} label={helpLabel} />
                ) : null}
              </div>
              {description ? (
                <div ref={descRef}>
                  <Lede>{description}</Lede>
                </div>
              ) : null}
            </div>
            {actions ? (
              <div
                data-slot="page-hero-actions"
                className={cn(
                  'flex min-w-0 basis-full flex-wrap items-center gap-2 lg:ml-auto lg:basis-auto lg:justify-end',
                  wrapped && 'lg:self-start',
                  wrapped && eyebrow && 'mt-5',
                )}
              >
                {actions}
              </div>
            ) : null}
          </div>
          {tabs ? <div className="border-b border-border">{tabs}</div> : null}
        </header>
      )}
      {banners ? <div className="space-y-2 empty:hidden">{banners}</div> : null}
      {aside ? (
        <div
          data-page-body=""
          className="grid grid-cols-1 items-start gap-(--glinr-space-6) @min-[80rem]/page:grid-cols-[minmax(0,1fr)_var(--glinr-aside-width)]"
        >
          <div
            data-page-main=""
            className="flex min-w-0 flex-col gap-(--glinr-page-gap)"
          >
            {children}
          </div>
          <aside
            data-page-aside=""
            aria-label={asideLabel}
            className="flex min-w-0 flex-col gap-(--glinr-page-gap) @min-[80rem]/page:sticky @min-[80rem]/page:top-(--glinr-aside-top)"
          >
            {aside}
          </aside>
        </div>
      ) : (
        children
      )}
    </div>
  )
}
