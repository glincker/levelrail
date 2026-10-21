import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { useRegisterPageTitle } from './shell/pageTitleStore'

// Keys carry their namespace prefix (e.g. "functions:banner.title").
// The banner is the page header: it registers the title and renders the h1.
export function PageBanner({
  icon,
  titleKey,
  descriptionKey,
  action,
  illustration,
  compact = false,
  className,
}: {
  icon?: ReactNode
  titleKey: string
  descriptionKey: string
  action?: ReactNode
  illustration?: ReactNode
  compact?: boolean
  className?: string
}) {
  const { t } = useTranslation()
  const title = t(titleKey, { defaultValue: titleKey })
  const description = t(descriptionKey, { defaultValue: descriptionKey })
  useRegisterPageTitle(title)

  const tile = icon ? (
    <span
      aria-hidden="true"
      className="flex size-(--glinr-hero-tile-size) shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground ring-1 ring-border [&_svg]:size-5"
    >
      {icon}
    </span>
  ) : null

  if (compact) {
    return (
      <header
        data-testid="page-banner-compact"
        className={cn(
          'flex flex-wrap items-center gap-3 rounded-xl border border-border bg-card px-4 py-3',
          className,
        )}
      >
        {tile}
        <div className="min-w-0 flex-1 basis-64">
          <h1 className="text-base font-semibold tracking-tight">{title}</h1>
          <p className="truncate text-sm text-muted-foreground">
            {description}
          </p>
        </div>
        {action ? (
          <div className="flex items-center gap-2">{action}</div>
        ) : null}
      </header>
    )
  }

  return (
    <header
      data-testid="page-banner-full"
      className={cn(
        'relative isolate flex items-stretch overflow-hidden rounded-xl border border-border bg-card',
        className,
      )}
    >
      <div className="relative z-10 flex min-w-0 flex-1 flex-col justify-center gap-4 px-6 py-8 md:max-w-sm md:flex-none lg:max-w-md">
        {tile}
        <div className="space-y-2">
          <h1 className="text-xl font-semibold tracking-tight">{title}</h1>
          <p className="text-sm text-muted-foreground">{description}</p>
        </div>
        {action ? (
          <div className="flex flex-wrap items-center gap-2">{action}</div>
        ) : null}
      </div>
      {illustration ? (
        <div
          aria-hidden="true"
          className="relative hidden min-h-64 flex-1 md:block"
        >
          <div className="absolute inset-0 bg-[radial-gradient(60%_55%_at_55%_0%,var(--iso-glow-warm),transparent),radial-gradient(60%_55%_at_45%_100%,var(--iso-glow-cool),transparent)] [mask-image:radial-gradient(ellipse_at_center,black_45%,transparent_100%)]" />
          <div className="absolute inset-0 flex items-center justify-center p-2 [mask-image:radial-gradient(ellipse_at_center,black_55%,transparent_100%)]">
            {illustration}
          </div>
        </div>
      ) : null}
    </header>
  )
}
