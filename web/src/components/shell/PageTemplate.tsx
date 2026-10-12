import type { ReactNode } from 'react'
import { HelpLink } from '../HelpLink'
import { Display, Eyebrow, Lede } from '@/components/ui/typography'
import { cn } from '@/lib/utils'

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
  /** 2xl by default; 3xl is for the dashboard greeting only. */
  titleSize?: '2xl' | '3xl'
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
  titleSize = '2xl',
  className,
  children,
}: PageTemplateProps) {
  const tile =
    logo ??
    (icon ? (
      <span
        aria-hidden="true"
        className="flex size-(--glinr-hero-tile-size) shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground ring-1 ring-border [&_svg]:size-5"
      >
        {icon}
      </span>
    ) : null)

  return (
    <div
      data-page-width={width}
      className={cn(
        'flex w-full flex-col gap-(--glinr-page-gap)',
        WIDTH[width],
        className,
      )}
    >
      <header className="space-y-3">
        {breadcrumb ? <div>{breadcrumb}</div> : null}
        <div className="flex flex-wrap items-start gap-3">
          {tile}
          <div className="min-w-0 flex-1 basis-64 space-y-1">
            {eyebrow ? <Eyebrow>{eyebrow}</Eyebrow> : null}
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
              <Display as="h1" size={titleSize} className="min-w-0 break-words">
                {title}
              </Display>
              {status}
              {helpPath && helpLabel ? (
                <HelpLink path={helpPath} label={helpLabel} />
              ) : null}
            </div>
            {description ? <Lede>{description}</Lede> : null}
          </div>
          {actions ? (
            <div className="flex flex-wrap items-center gap-2 sm:ml-auto">
              {actions}
            </div>
          ) : null}
        </div>
        {tabs ? <div className="border-b border-border">{tabs}</div> : null}
      </header>
      {banners ? <div className="space-y-2">{banners}</div> : null}
      {children}
    </div>
  )
}
