import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { useRegisterPageTitle } from './shell/pageTitleStore'

export type PageBannerSize = 'hero' | 'compact'

const EDGE_FADE =
  '[mask-image:radial-gradient(ellipse_at_center,black_55%,transparent_100%)]'
const GLOW =
  'bg-[radial-gradient(60%_55%_at_55%_0%,var(--iso-glow-warm),transparent),radial-gradient(60%_55%_at_45%_100%,var(--iso-glow-cool),transparent)]'

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

  const tile = icon ? (
    <span
      aria-hidden="true"
      className="flex size-(--glinr-hero-tile-size) shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground ring-1 ring-border [&_svg]:size-5"
    >
      {icon}
    </span>
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
      data-testid={compact ? 'page-banner-compact' : 'page-banner-full'}
      className={cn(
        'relative isolate flex items-stretch overflow-hidden rounded-xl border border-border bg-card',
        className,
      )}
    >
      <div
        className={cn(
          'relative z-10 flex min-w-0 flex-1 gap-x-3',
          compact
            ? 'min-h-[4.5rem] flex-wrap items-center px-4 py-3'
            : 'flex-col justify-center gap-y-3 px-6 py-5 md:max-w-sm md:flex-none lg:max-w-md',
        )}
      >
        <div
          className={cn(
            'flex min-w-0 items-center gap-3',
            compact && 'flex-1 basis-64',
          )}
        >
          {tile}
          <div className="min-w-0">
            <h1
              className={cn(
                'font-semibold tracking-tight',
                compact ? 'text-base' : 'text-xl',
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
          <p className="line-clamp-2 text-sm text-muted-foreground">
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
