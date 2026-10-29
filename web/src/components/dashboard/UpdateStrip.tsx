import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  ArrowRightGlassIcon,
  ArrowUpGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
import { Button, buttonVariants } from '@/components/ui/button'
import { TONE, type Tone } from '@/components/kit/tone'
import { cn } from '@/lib/utils'
import { useIsRoot } from '../../hooks/useIsRoot'
import { updatesQueryOptions, type UpdateStatus } from '../../queries/updates'
import {
  behindWarnThreshold,
  displayVersion,
  releasesBehind,
} from '../../lib/updateBehind'
import { dismissUpdate, useUpdateDismissed } from '../../lib/updateDismiss'

// The API has no security flag yet; this shows it as soon as the field exists.
type UpdateInfo = UpdateStatus & { security_update?: boolean }

function updateTone(behind: number | null, security: boolean): Tone {
  if (security) return 'warning'
  return behind !== null && behind >= behindWarnThreshold()
    ? 'warning'
    : 'neutral'
}

// Only https release links are trusted as an href; anything else falls back to the in-app page.
function safeReleaseUrl(raw: string | null | undefined): string | null {
  if (!raw) return null
  try {
    const u = new URL(raw)
    return u.protocol === 'https:' ? u.href : null
  } catch {
    return null
  }
}

export function UpdateStripView({
  info,
  canUpdate,
}: {
  info: UpdateInfo
  canUpdate: boolean
}) {
  const { t } = useTranslation('dashboard')
  const latest = info.latest_version ?? ''
  const dismissed = useUpdateDismissed(latest)
  const security = info.security_update === true
  if (!info.update_available || !latest) return null
  if (dismissed && !security) return null
  const behind = releasesBehind(info.current_version, latest)
  const tone = updateTone(behind, security)
  const releaseHref = safeReleaseUrl(info.release_url)
  return (
    <section
      aria-label={t('updateStrip.label')}
      data-tone={tone}
      className={cn(
        'flex flex-wrap items-center gap-x-3 gap-y-2 rounded-lg border px-4 py-2',
        TONE[tone].soft,
        TONE[tone].border,
      )}
    >
      <span
        className={cn(
          'flex items-center gap-2 text-sm font-medium',
          TONE[tone].text,
        )}
      >
        <ArrowUpGlassIcon aria-hidden="true" className="size-5" />
        <span className="flex items-center gap-1.5 tabular-nums">
          {displayVersion(info.current_version)}
          <ArrowRightGlassIcon aria-hidden="true" className="size-5" />
          <span className="sr-only">{t('updateStrip.to')}</span>
          {displayVersion(latest)}
        </span>
      </span>
      {behind !== null && behind > 1 ? (
        <span className="text-xs text-muted-foreground">
          {t('updateStrip.behind', { count: behind })}
        </span>
      ) : null}
      {security ? (
        <span className={cn('text-xs font-medium', TONE.warning.text)}>
          {t('updateStrip.security')}
        </span>
      ) : null}
      <span className="ml-auto flex items-center gap-1.5">
        {releaseHref ? (
          <a
            href={releaseHref}
            target="_blank"
            rel="noreferrer"
            className={buttonVariants({ size: 'sm', variant: 'ghost' })}
          >
            {t('updateStrip.whatsNew')}
          </a>
        ) : (
          <Link
            to="/settings/updates"
            className={buttonVariants({ size: 'sm', variant: 'ghost' })}
          >
            {t('updateStrip.whatsNew')}
          </Link>
        )}
        {canUpdate ? (
          <Link
            to="/settings/updates"
            className={buttonVariants({ size: 'sm', variant: 'outline' })}
          >
            {t('updateStrip.update')}
          </Link>
        ) : null}
        {security ? null : (
          <Button
            size="icon-sm"
            variant="ghost"
            aria-label={t('updateStrip.dismiss', {
              version: displayVersion(latest),
            })}
            onClick={() => {
              dismissUpdate(latest)
            }}
          >
            <XGlassIcon size="1.4em" aria-hidden="true" />
          </Button>
        )}
      </span>
    </section>
  )
}

/** UpdateStrip is the dashboard's one primary place for "a new release exists", kept apart from the health line. */
export function UpdateStrip() {
  const isRoot = useIsRoot()
  const updates = useQuery({
    ...updatesQueryOptions(),
    retry: false,
    staleTime: 60 * 60_000,
  })
  if (!updates.data) return null
  return <UpdateStripView info={updates.data} canUpdate={isRoot} />
}
