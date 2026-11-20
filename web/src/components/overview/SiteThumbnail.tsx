import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { GlobeGlassIcon } from '@thesvg/ui-react'
import { RelativeTime } from '@/components/kit'

function faviconOf(url: string): string | undefined {
  try {
    return `${new URL(url).origin}/favicon.ico`
  } catch {
    return undefined
  }
}

function hostOf(url: string): string {
  try {
    return new URL(url).host
  } catch {
    return url.replace(/^https?:\/\//, '')
  }
}

/** Stand-in for the live frame when the site cannot be framed: a calm site card, never an error. */
export function SiteThumbnail({
  url,
  title,
  deployedAt,
}: {
  url: string
  title: string
  deployedAt?: string
}) {
  const { t } = useTranslation('appOverview')
  const [iconFailed, setIconFailed] = useState(false)
  const icon = faviconOf(url)
  return (
    <div
      role="img"
      aria-label={t('preview.thumbLabel', { name: title, host: hostOf(url) })}
      className="absolute inset-0 flex flex-col bg-gradient-to-br from-muted to-background"
    >
      <div className="flex items-center gap-2 border-b border-border bg-card/70 px-3 py-2">
        <span className="flex gap-1" aria-hidden="true">
          <span className="size-2 rounded-full bg-border" />
          <span className="size-2 rounded-full bg-border" />
          <span className="size-2 rounded-full bg-border" />
        </span>
        <span className="min-w-0 flex-1 truncate rounded bg-muted px-2 py-0.5 text-center text-xs text-muted-foreground">
          {hostOf(url)}
        </span>
      </div>
      <div className="flex flex-1 flex-col items-center justify-center gap-2 px-4 text-center">
        {icon && !iconFailed ? (
          <img
            src={icon}
            alt=""
            width={40}
            height={40}
            className="size-10 rounded-lg border border-border bg-card object-contain p-1"
            onError={() => setIconFailed(true)}
          />
        ) : (
          <span className="flex size-10 items-center justify-center rounded-lg border border-border bg-card text-muted-foreground">
            <GlobeGlassIcon className="size-6" aria-hidden="true" />
          </span>
        )}
        <p className="max-w-full truncate text-base font-semibold">{title}</p>
        {deployedAt ? (
          <p className="text-xs text-muted-foreground">
            {t('preview.deployedAgo')} <RelativeTime at={deployedAt} />
          </p>
        ) : null}
      </div>
    </div>
  )
}
