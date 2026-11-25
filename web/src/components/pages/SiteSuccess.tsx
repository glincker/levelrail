import { useEffect, useRef } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  CopyGlassIcon,
  ExternalLinkGlassIcon,
  GitBranchGlassIcon,
  GlobeGlassIcon,
  LockGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { useAbilities } from '../../hooks/useAbilities'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import { formatDurationMs } from '../../lib/deployDuration'
import { appDetailQueryOptions } from '../../queries/apps'
import { pagesSiteDetailQueryOptions } from '../../queries/pagesSites'
import { visibleSuccessChips, type SuccessChipId } from './successChips'

/** Live URL as a chip with Open and Copy. Renders a no-domain hint when the site has none yet. */
export function SiteUrlChip({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('pagesSites')
  const { copied, copy } = useCopyToClipboard()
  const app = useQuery({ ...appDetailQueryOptions(name), retry: false })
  const domain = app.data?.domains?.[0]
  if (!domain) {
    return (
      <span className="text-xs text-muted-foreground">{t('list.noUrl')}</span>
    )
  }
  const url = `https://${domain}`
  return (
    <span className="inline-flex max-w-full items-center gap-1 rounded-full border border-border bg-muted/50 py-0.5 pr-1 pl-3 text-sm">
      <span className="truncate font-mono text-xs">{domain}</span>
      <Button
        size="xs"
        variant="ghost"
        render={<a href={url} target="_blank" rel="noreferrer" />}
      >
        {t('success.open')}
      </Button>
      <Button
        size="icon-sm"
        variant="ghost"
        aria-label={t('success.copy')}
        onClick={() => copy(url)}
      >
        {copied ? (
          <CircleCheckGlassIcon className="size-4" aria-hidden="true" />
        ) : (
          <CopyGlassIcon className="size-4" aria-hidden="true" />
        )}
      </Button>
    </span>
  )
}

function ChipIcon({ id }: Readonly<{ id: SuccessChipId }>) {
  const cls = 'size-3.5'
  if (id === 'domain') {
    return <GlobeGlassIcon className={cls} aria-hidden="true" />
  }
  if (id === 'git') {
    return <GitBranchGlassIcon className={cls} aria-hidden="true" />
  }
  return <LockGlassIcon className={cls} aria-hidden="true" />
}

/** Big live URL with Copy (focused once the URL is known) and Open, shown right after a publish. */
function LiveUrl({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('pagesSites')
  const { copied, copy } = useCopyToClipboard()
  const copyRef = useRef<HTMLButtonElement>(null)
  const app = useQuery({ ...appDetailQueryOptions(name), retry: false })
  const domain = app.data?.domains?.[0]
  useEffect(() => {
    if (domain) copyRef.current?.focus()
  }, [domain])
  if (!domain) {
    return (
      <span className="text-xs text-muted-foreground">{t('list.noUrl')}</span>
    )
  }
  const url = `https://${domain}`
  return (
    <div className="flex items-center gap-1.5 rounded-lg border border-border bg-muted/50 py-1.5 pr-1.5 pl-3">
      <a
        href={url}
        target="_blank"
        rel="noreferrer"
        className="min-w-0 flex-1 truncate font-mono text-base font-medium hover:underline focus-visible:underline"
      >
        {domain}
      </a>
      <Button
        ref={copyRef}
        size="sm"
        variant="outline"
        onClick={() => copy(url)}
      >
        {copied ? (
          <CircleCheckGlassIcon className="size-4" aria-hidden="true" />
        ) : (
          <CopyGlassIcon className="size-4" aria-hidden="true" />
        )}
        {copied ? t('success.copied') : t('success.copy')}
      </Button>
      <Button
        size="sm"
        variant="ghost"
        render={<a href={url} target="_blank" rel="noreferrer" />}
      >
        <ExternalLinkGlassIcon className="size-4" aria-hidden="true" />
        {t('success.open')}
      </Button>
      <span className="sr-only" role="status" aria-live="polite">
        {copied ? t('success.copiedAnnounce', { domain }) : ''}
      </span>
    </div>
  )
}

/** Shown in place of the form once a publish lands: live URL, deploy time and next-step chips the user may act on. */
export function SiteSuccess({
  name,
  onNavigate,
}: Readonly<{ name: string; onNavigate?: () => void }>) {
  const { t } = useTranslation('pagesSites')
  const { can } = useAbilities()
  // The cached detail may predate this deploy, so always refetch for the real time.
  const detail = useQuery({
    ...pagesSiteDetailQueryOptions(name),
    staleTime: 0,
  }).data
  const deploy = detail?.latest_deploy
  const took =
    deploy?.finished_at && deploy.started_at
      ? formatDurationMs(
          new Date(deploy.finished_at).getTime() -
            new Date(deploy.started_at).getTime(),
        )
      : null
  const chips = visibleSuccessChips(can)
  return (
    <div className="space-y-3">
      <p
        className="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-sm font-medium"
        role="status"
      >
        <span className="flex items-center gap-1.5">
          <CircleCheckGlassIcon
            className="size-4 text-success"
            aria-hidden="true"
          />
          {t('success.title', { name })}
        </span>
        {took ? (
          <span className="text-xs font-normal text-muted-foreground">
            {t('success.took', { time: took })}
          </span>
        ) : null}
      </p>
      <LiveUrl name={name} />
      {chips.length > 0 ? (
        <div className="flex flex-wrap items-center gap-2 text-xs">
          <span className="text-muted-foreground">{t('success.next')}</span>
          {chips.map((c) => (
            <Button
              key={c.id}
              size="xs"
              variant="outline"
              onClick={onNavigate}
              render={
                <Link
                  to="/pages/$name"
                  params={{ name }}
                  search={c.tab === 'deployments' ? {} : { tab: c.tab }}
                />
              }
            >
              <ChipIcon id={c.id} />
              {t(`success.chip.${c.id}`)}
            </Button>
          ))}
        </div>
      ) : null}
    </div>
  )
}
