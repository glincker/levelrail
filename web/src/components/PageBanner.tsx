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

  // The icon keeps its true shape: depth comes from a rim light, contact and cast shadows and a floor shadow, never a perspective warp.
  const iconArt = icon ? (
    <div
      aria-hidden="true"
      className={cn(
        'pointer-events-none absolute z-[5] hidden -translate-y-1/2 sm:block',
        compact ? 'top-1/2 left-4 size-14' : 'top-1/2 left-6 size-24',
      )}
    >
      <div className="absolute inset-x-2 -bottom-3 h-3 rounded-full bg-black/45 blur-md" />
      <div
        className={cn(
          'absolute inset-0 -rotate-6 [&_svg]:size-full [&_svg]:text-(--banner-icon)!',
          'drop-shadow-[0_-1px_0_rgba(255,255,255,0.55)] drop-shadow-[0_2px_2px_rgba(0,0,0,0.35)] drop-shadow-[0_14px_14px_rgba(0,0,0,0.3)]',
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
        'relative isolate flex items-stretch overflow-hidden rounded-3xl border border-border/80 bg-[linear-gradient(180deg,color-mix(in_oklab,var(--card),var(--foreground)_4%),var(--card))] shadow-[inset_0_1px_0_0_rgba(255,255,255,0.08),inset_0_-1px_0_0_rgba(0,0,0,0.25),0_1px_2px_rgba(0,0,0,0.25),0_22px_44px_-22px_rgba(0,0,0,0.6)] [&_[data-slot=help-link]]:absolute [&_[data-slot=help-link]]:right-3 [&_[data-slot=help-link]]:bottom-3 [&_[data-slot=help-link]]:z-20',
        className,
      )}
    >
      {icon ? (
        <div
          aria-hidden="true"
          className={cn(
            'grain-orb pointer-events-none absolute -z-10 hidden sm:block',
            compact ? '-top-6 -left-8 size-40' : '-top-10 -left-10 size-60',
          )}
        />
      ) : null}
      {iconArt}
      <div
        className={cn(
          'z-10 flex min-w-0 flex-1 gap-x-3',
          compact
            ? 'min-h-[4.5rem] flex-wrap items-center px-4 py-3'
            : 'min-h-28 flex-col justify-center gap-y-2 px-6 py-5 md:max-w-xl md:flex-none',
          icon && (compact ? 'sm:pl-24' : 'sm:pl-40'),
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
                compact ? 'text-xl' : 'text-2xl md:text-3xl',
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
          <p className="line-clamp-2 max-w-prose text-sm text-muted-foreground">
            {description}
          </p>
        )}
      </div>
      {action ? (
        <div className="z-10 ml-auto flex shrink-0 flex-wrap items-center justify-end gap-2 self-center px-5 py-4">
          {action}
        </div>
      ) : null}
    </header>
  )
}
