import * as React from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArrowUpGlassIcon,
  CheckGlassIcon,
  ChevronDownGlassIcon,
  ExternalLinkGlassIcon,
  SparklesGlassIcon,
} from '@thesvg/ui-react'
import { Button, buttonVariants } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import { toast, toastAction } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import { useIsRoot } from '../../hooks/useIsRoot'
import { changelogQueryOptions } from '../../queries/changelog'
import { updatesQueryOptions } from '../../queries/updates'
import { displayVersion } from '../../lib/updateBehind'
import { CountBadge } from './CountBadge'
import { WhatsNewCard } from './WhatsNewCard'
import {
  markUpgradeToasted,
  onWhatsNewOpenRequest,
  requestOpenWhatsNew,
  setLastSeenVersion,
  shouldToastUpgrade,
  unreadCount,
  useLastSeenVersion,
} from './whatsNew'
import { groupBullets, pickHighlights } from './whatsNewHighlights'

// Only https release links are trusted as an href.
function safeReleaseUrl(raw: string | null | undefined): string | null {
  if (!raw) return null
  try {
    const u = new URL(raw)
    return u.protocol === 'https:' ? u.href : null
  } catch {
    return null
  }
}

// Bell-style unread badge with a distinct sparkle glyph so it reads apart
// from the operational notification center.
export function WhatsNewPanel() {
  const { t } = useTranslation('common', { useSuspense: false })
  const isRoot = useIsRoot()
  const changelog = useQuery(changelogQueryOptions())
  const updates = useQuery(updatesQueryOptions())
  const lastSeen = useLastSeenVersion()
  const [open, setOpen] = React.useState(false)
  const [showAll, setShowAll] = React.useState(false)
  const triggerRef = React.useRef<HTMLButtonElement>(null)
  const entries = changelog.data?.entries ?? []
  const latest = entries[0]
  const unread = unreadCount(entries, lastSeen)

  // The header and its overflow menu both mount a panel; only the visible one opens.
  React.useEffect(
    () =>
      onWhatsNewOpenRequest(() => {
        const el = triggerRef.current
        if (el && el.getClientRects().length > 0) setOpen(true)
      }),
    [],
  )

  const latestVersion = latest?.version ?? ''
  React.useEffect(() => {
    if (!shouldToastUpgrade(latestVersion, lastSeen)) return
    markUpgradeToasted(latestVersion)
    toast.add({
      title: t('shell.whatsNew.toast.title', {
        version: displayVersion(latestVersion),
      }),
      description: t('shell.whatsNew.toast.body'),
      type: 'info',
      actionProps: toastAction(t('shell.whatsNew.toast.open'), () => {
        requestOpenWhatsNew()
      }),
    })
  }, [latestVersion, lastSeen, t])

  const split = React.useMemo(
    () => pickHighlights(latest?.bullets ?? []),
    [latest],
  )
  const groups = React.useMemo(() => groupBullets(split.rest), [split.rest])
  const close = () => {
    setOpen(false)
  }
  const updateInfo = updates.data
  const releaseHref = safeReleaseUrl(updateInfo?.release_url)

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) {
          setShowAll(false)
          if (latest) setLastSeenVersion(latest.version)
        }
      }}
    >
      <PopoverTrigger
        render={
          <Button
            ref={triggerRef}
            type="button"
            variant="outline"
            size="icon-sm"
            className="relative"
            aria-label={
              unread > 0
                ? t('shell.whatsNew.unread', { count: unread })
                : t('shell.whatsNew.allCaughtUp')
            }
            aria-expanded={open}
            title={t('shell.whatsNew.title')}
          />
        }
      >
        <SparklesGlassIcon size="1.4em" />
        {unread > 0 ? <CountBadge count={unread} tone="primary" /> : null}
      </PopoverTrigger>
      <PopoverContent
        align="end"
        aria-label={t('shell.whatsNew.title')}
        className="max-h-[min(36rem,calc(100dvh-5rem))] w-[min(26rem,calc(100vw-2rem))] min-w-[min(20rem,calc(100vw-2rem))] overflow-y-auto overscroll-contain"
      >
        <PopoverHeader className="gap-1.5">
          <div className="flex items-center justify-between gap-2">
            <PopoverTitle>{t('shell.whatsNew.title')}</PopoverTitle>
            {unread > 0 && latest ? (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="cursor-pointer"
                onClick={() => {
                  setLastSeenVersion(latest.version)
                }}
              >
                <CheckGlassIcon size="1.2em" aria-hidden="true" />
                {t('shell.whatsNew.markSeen')}
              </Button>
            ) : null}
          </div>
          {latest ? (
            <p className="text-xs text-muted-foreground tabular-nums">
              {displayVersion(latest.version)} · {latest.date}
            </p>
          ) : null}
        </PopoverHeader>

        {changelog.isError ? (
          <div className="flex flex-col items-start gap-2 px-1 py-3">
            <p className="text-sm text-muted-foreground">
              {t('shell.whatsNew.error')}
            </p>
            <Button
              type="button"
              variant="outline"
              className="h-10 cursor-pointer"
              onClick={() => {
                void changelog.refetch()
              }}
            >
              {t('shell.whatsNew.retry')}
            </Button>
          </div>
        ) : !latest ? (
          <p className="px-2 py-6 text-center text-sm text-muted-foreground">
            {changelog.isPending
              ? t('shell.whatsNew.loading')
              : t('shell.whatsNew.empty')}
          </p>
        ) : (
          <>
            <div
              className={cn(
                'flex flex-wrap items-center gap-2 rounded-lg border px-3 py-2',
                updateInfo?.update_available
                  ? 'border-tone-info-border bg-tone-info-soft'
                  : 'bg-muted/40',
              )}
            >
              <p className="min-w-0 flex-1 text-sm">
                {updateInfo?.update_available && updateInfo.latest_version
                  ? t('shell.whatsNew.updateAvailable', {
                      version: displayVersion(updateInfo.latest_version),
                    })
                  : t('shell.whatsNew.onLatest')}
              </p>
              {updateInfo?.update_available && isRoot ? (
                <Link
                  to="/settings/updates"
                  onClick={close}
                  className={cn(
                    buttonVariants({ variant: 'default' }),
                    'h-10 cursor-pointer max-sm:w-full',
                  )}
                >
                  <ArrowUpGlassIcon size="1.2em" aria-hidden="true" />
                  {t('shell.whatsNew.update')}
                </Link>
              ) : null}
              {releaseHref ? (
                <a
                  href={releaseHref}
                  target="_blank"
                  rel="noreferrer"
                  className="inline-flex min-h-10 cursor-pointer items-center gap-1 rounded-md px-1 text-xs text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
                >
                  {t('shell.whatsNew.releaseNotes')}
                  <ExternalLinkGlassIcon size="1.1em" aria-hidden="true" />
                </a>
              ) : null}
            </div>

            {split.highlights.length > 0 ? (
              <ul
                aria-label={t('shell.whatsNew.highlights')}
                className="flex flex-col gap-2"
              >
                {split.highlights.map((h) => (
                  <WhatsNewCard key={h.index} item={h} onNavigate={close} />
                ))}
              </ul>
            ) : null}

            {split.rest.length > 0 ? (
              <div className="flex flex-col gap-2">
                <Button
                  type="button"
                  variant="ghost"
                  className="h-10 cursor-pointer justify-between px-2"
                  aria-expanded={showAll}
                  aria-controls="whatsnew-all"
                  onClick={() => {
                    setShowAll((v) => !v)
                  }}
                >
                  {t('shell.whatsNew.allChanges', {
                    count: latest.bullets.length,
                  })}
                  <ChevronDownGlassIcon
                    size="1.2em"
                    aria-hidden="true"
                    className={cn(
                      'transition-transform motion-reduce:transition-none',
                      showAll && 'rotate-180',
                    )}
                  />
                </Button>
                {showAll ? (
                  <div id="whatsnew-all" className="flex flex-col gap-3 px-2">
                    {groups.map((g) => (
                      <section key={g.group}>
                        <h3 className="text-xs font-semibold text-muted-foreground">
                          {t(`shell.whatsNew.groups.${g.group}`)}
                        </h3>
                        <ul className="mt-1 space-y-1 text-sm">
                          {g.items.map((b) => (
                            <li key={b.index} className="break-words">
                              {b.text}
                            </li>
                          ))}
                        </ul>
                      </section>
                    ))}
                  </div>
                ) : null}
              </div>
            ) : null}
          </>
        )}
      </PopoverContent>
    </Popover>
  )
}
