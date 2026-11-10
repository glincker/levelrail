import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { useRegisterPageTitle } from './shell/pageTitleStore'

export type PageBannerSize = 'hero' | 'compact'

// Keys carry their namespace prefix (e.g. "functions:banner.title").
// The banner is the page header: it registers the title and renders the h1.
export function PageBanner({
  icon,
  titleKey,
  titleText,
  descriptionKey,
  descriptionText,
  action,
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
  // Art is reserved for the hero size; compact is a plain title row.
  const art = !compact && Boolean(icon)

  // The icon keeps its true shape: depth comes from a rim light, contact and cast shadows and a floor shadow, never a perspective warp.
  const iconArt = art ? (
    <div
      aria-hidden="true"
      className={cn(
        'pointer-events-none absolute z-[5] hidden -translate-y-1/2 sm:block motion-safe:animate-in motion-safe:fade-in-0 motion-safe:zoom-in-90 motion-safe:duration-500',
        'top-1/2 left-5 size-14',
      )}
    >
      <div
        className={cn(
          'absolute inset-0 [&_svg]:size-full [&_svg]:text-(--banner-icon)!',
          'drop-shadow-[0_1px_1px_rgba(0,0,0,0.25)] drop-shadow-[0_6px_8px_rgba(0,0,0,0.18)]',
        )}
      >
        {icon}
      </div>
    </div>
  ) : null

  return (
    <header
      data-testid={compact ? 'page-banner-compact' : 'page-banner-full'}
      className={cn(
        'relative isolate flex flex-col items-stretch overflow-hidden sm:flex-row sm:flex-wrap',
        compact
          ? 'rounded-xl border border-border bg-card'
          : 'rounded-2xl border border-border bg-card sm:flex-nowrap sm:items-center',
        className,
      )}
    >
      {iconArt}
      <div
        className={cn(
          'z-10 flex min-w-0 flex-1 gap-x-3',
          compact
            ? 'min-h-[4.5rem] flex-wrap items-center px-4 py-3'
            : 'min-h-20 flex-col justify-center gap-y-1 px-5 py-4',
          art && 'sm:pl-24',
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
                'font-(family-name:--font-display) font-light tracking-[-0.03em] leading-[1.08] text-foreground',
                'text-xl',
              )}
            >
              {title}
            </h1>
            {compact ? (
              <p className="line-clamp-3 text-sm text-muted-foreground md:line-clamp-1">
                {description}
              </p>
            ) : null}
          </div>
        </div>
        {compact ? null : (
          <p className="line-clamp-2 max-w-prose text-sm text-muted-foreground">
            {description}
          </p>
        )}
      </div>
      {action ? (
        <div
          className={cn(
            'z-10 flex shrink-0 flex-wrap items-center gap-2 px-4 pb-4 sm:ml-auto sm:justify-end sm:self-center sm:px-5 sm:py-4',
          )}
        >
          {action}
        </div>
      ) : null}
    </header>
  )
}
