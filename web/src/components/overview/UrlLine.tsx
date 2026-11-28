import {
  CheckGlassIcon,
  CopyGlassIcon,
  ExternalLinkGlassIcon,
  GlobeGlassIcon,
} from '@thesvg/ui-react'
import { Link } from '@tanstack/react-router'
import { Button } from '@/components/ui/button'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import type { UrlReachability, UrlState } from '../../lib/urlReachability'
import { AppQrButton } from '../apps/AppQrButton'

const CHIP_VARIANT = {
  down: 'warning',
  waiting_dns: 'info',
  local_only: 'muted',
  unknown: 'muted',
} as const satisfies Record<Exclude<UrlState, 'reachable'>, string>

export function UrlLine({
  appName,
  url,
  reach,
  shareUrl = null,
}: {
  appName: string
  url: string | null
  reach?: UrlReachability
  shareUrl?: string | null
}) {
  const { copied, copy } = useCopyToClipboard()
  const { t } = useTranslation('appOverview')
  const live =
    !reach || reach.state === 'reachable' || reach.state === 'local_only'
  const reason =
    reach && reach.state !== 'reachable'
      ? t(`status.reason.${reach.state}`)
      : undefined
  if (!url) {
    return (
      <p className="flex items-center gap-2 text-sm text-muted-foreground">
        <GlobeGlassIcon className="size-5" aria-hidden="true" />
        No public URL yet.
        <Link
          to="/apps/$name/domains"
          params={{ name: appName }}
          className="text-primary underline underline-offset-2"
        >
          Add a domain
        </Link>
      </p>
    )
  }
  return (
    <div className="flex min-w-0 items-center gap-1">
      {live ? (
        <a
          href={url}
          target="_blank"
          rel="noreferrer"
          className="inline-flex min-w-0 items-center gap-1.5 truncate text-sm font-medium text-primary hover:underline"
        >
          <GlobeGlassIcon className="size-5 shrink-0" aria-hidden="true" />
          <span className="truncate">{url.replace(/^https?:\/\//, '')}</span>
          <ExternalLinkGlassIcon
            className="size-5 shrink-0"
            aria-hidden="true"
          />
        </a>
      ) : (
        <span
          title={reason}
          className="inline-flex min-w-0 items-center gap-1.5 truncate text-sm text-muted-foreground"
        >
          <GlobeGlassIcon className="size-5 shrink-0" aria-hidden="true" />
          <span className="truncate">{url.replace(/^https?:\/\//, '')}</span>
        </span>
      )}
      {reach && reach.state !== 'reachable' ? (
        <Badge
          variant={CHIP_VARIANT[reach.state]}
          title={reason}
          className="shrink-0"
        >
          {t(`status.chip.${reach.state}`)}
        </Badge>
      ) : null}
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={copied ? 'URL copied' : 'Copy URL'}
        onClick={() => copy(url)}
      >
        {copied ? (
          <CheckGlassIcon className="size-5" aria-hidden="true" />
        ) : (
          <CopyGlassIcon className="size-5" aria-hidden="true" />
        )}
      </Button>
      {reach ? (
        <AppQrButton
          appName={appName}
          url={url}
          shareUrl={shareUrl}
          reach={reach}
        />
      ) : null}
    </div>
  )
}
