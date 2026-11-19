import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  EllipsisGlassIcon,
  LockGlassIcon,
  UploadGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLinkItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { formatAge, formatSize } from '../../lib/format'
import { usePagesSiteDetail, useSiteSettings } from '../../queries/pagesSites'
import { SiteUrlChip } from './SiteSuccess'
import { siteState, type SiteState } from './pageHelpers'

const DOT: Record<SiteState, string> = {
  live: 'bg-success',
  publishing: 'bg-warning motion-safe:animate-pulse',
  notPublished: 'bg-muted-foreground/50',
}

export function StatusDot({ state }: Readonly<{ state: SiteState }>) {
  const { t } = useTranslation('pagesSites')
  return (
    <span className="inline-flex items-center gap-1.5 text-sm font-medium">
      <span
        aria-hidden="true"
        className={`size-2 rounded-full transition-colors duration-200 ${DOT[state]}`}
      />
      <span role="status">{t(`hero.status.${state}`)}</span>
    </span>
  )
}

/** Header strip: state, live URL, size and last deploy, then the primary publish action and a quiet overflow menu. */
export function SiteHero({
  name,
  onPublish,
  onDelete,
}: Readonly<{ name: string; onPublish: () => void; onDelete: () => void }>) {
  const { t } = useTranslation('pagesSites')
  const detail = usePagesSiteDetail(name).data
  const locked = useSiteSettings(name).data?.protected === true
  const state = siteState(detail)
  const lastAt =
    detail?.latest_deploy?.finished_at ??
    detail?.latest_deploy?.started_at ??
    detail?.updated_at
  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
      {state ? <StatusDot state={state} /> : null}
      {locked ? (
        <span
          role="img"
          title={t('settings.locked')}
          aria-label={t('settings.locked')}
          className="inline-flex items-center text-muted-foreground"
        >
          <LockGlassIcon className="size-4" aria-hidden="true" />
        </span>
      ) : null}
      <SiteUrlChip name={name} />
      <p className="flex items-center gap-x-3 text-xs text-muted-foreground tabular-nums">
        {detail ? (
          <span>
            {formatSize(detail.size_bytes)}
            {detail.file_count > 0
              ? ` · ${t('detail.files', { count: detail.file_count })}`
              : ''}
          </span>
        ) : null}
        {lastAt ? (
          <span>{t('detail.lastDeploy', { when: formatAge(lastAt) })}</span>
        ) : null}
      </p>
      <div className="ml-auto flex items-center gap-1.5">
        <Button size="sm" onClick={onPublish}>
          <UploadGlassIcon className="size-4" aria-hidden="true" />
          {t('detail.publish')}
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                size="icon-sm"
                variant="ghost"
                aria-label={t('hero.more')}
              />
            }
          >
            <EllipsisGlassIcon className="size-4" aria-hidden="true" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="min-w-44">
            <DropdownMenuLinkItem
              render={<Link to="/apps/$name" params={{ name }} />}
            >
              {t('settings.openApp')}
            </DropdownMenuLinkItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem className="text-tone-danger" onClick={onDelete}>
              {t('settings.delete')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  )
}
