import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ExternalLinkGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { BrandIcon } from '@/components/BrandIcon'
import { TONE } from '@/components/kit/tone'
import { brandForProvider } from '../../lib/deliveryBrand'
import { categoryOf, parseCommitStatus } from '../../lib/deliveryTimeline'
import { formatAbsoluteTime, formatRelativeTime } from '../../lib/relativeTime'
import type { TimelineItem } from '../../types/appDelivery'
import { statusKey, statusLook } from '../../lib/deliveryStatus'
import { StatusIcon } from './deliveryStatus'

function ExternalLink({ url, label }: { url: string; label: string }) {
  return (
    <a
      href={url}
      target="_blank"
      rel="noopener noreferrer"
      className="inline-flex items-center gap-1 rounded-sm text-xs font-medium text-primary underline-offset-2 hover:underline focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
    >
      {label}
      <ExternalLinkGlassIcon aria-hidden="true" className="size-3" />
    </a>
  )
}

const LINK_CLASS =
  'inline-flex items-center gap-1 rounded-sm text-xs font-medium text-primary underline-offset-2 hover:underline focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none'

function CommitStatusBody({ item }: { item: TimelineItem }) {
  const { t } = useTranslation('delivery')
  const info = parseCommitStatus(item)
  const brand = brandForProvider(info.provider)
  const postFailed = item.status === 'failed'
  return (
    <div className="mt-1 space-y-1.5">
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
        {info.provider ? (
          <span className="inline-flex items-center gap-1">
            {brand ? <BrandIcon name={brand} className="size-3.5" /> : null}
            <span>{info.provider}</span>
          </span>
        ) : null}
        {info.sha ? (
          <code className="rounded bg-muted px-1 py-0.5 font-mono text-[0.7rem] text-foreground">
            {info.sha}
          </code>
        ) : null}
        {info.description ? (
          <span className="min-w-0 break-words">{info.description}</span>
        ) : null}
        {item.url ? (
          <ExternalLink
            url={item.url}
            label={
              item.actor === 'preview' ? t('row.openPreview') : t('row.openRun')
            }
          />
        ) : null}
      </div>
      {postFailed ? (
        <details className="group text-xs">
          <summary
            className={`inline-flex cursor-pointer items-center gap-1 rounded-sm font-medium focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none ${TONE.danger.text}`}
          >
            <TriangleAlertGlassIcon aria-hidden="true" className="size-3.5" />
            {t('row.postFailed')}
          </summary>
          <p
            className={`mt-1 rounded-md border px-2 py-1.5 font-mono break-words ${TONE.danger.soft} ${TONE.danger.border}`}
          >
            {info.error || t('row.noErrorDetail')}
          </p>
        </details>
      ) : null}
    </div>
  )
}

function Extras({ item, app }: { item: TimelineItem; app: string }) {
  const { t } = useTranslation('delivery')
  if (item.kind === 'commit_status') return <CommitStatusBody item={item} />
  const links: React.ReactNode[] = []
  if (item.kind === 'connection_bound') {
    links.push(
      <Link key="int" to="/settings/integrations" className={LINK_CLASS}>
        {t('row.openIntegrations')}
      </Link>,
    )
  }
  if (item.ref?.type === 'deploy_attempt') {
    links.push(
      <Link
        key="run"
        to="/apps/$name/deploys/$deployId/logs"
        params={{ name: app, deployId: item.ref.id }}
        className={LINK_CLASS}
      >
        {t('row.openRun')}
      </Link>,
    )
  }
  if (item.url && item.kind !== 'commit_status') {
    links.push(
      <ExternalLink key="url" url={item.url} label={t('row.openLink')} />,
    )
  }
  return (
    <>
      {item.detail ? (
        <p className="mt-1 text-xs break-words text-muted-foreground">
          {item.detail}
        </p>
      ) : null}
      {links.length > 0 ? (
        <div className="mt-1 flex flex-wrap gap-3">{links}</div>
      ) : null}
    </>
  )
}

export function DeliveryItemRow({
  item,
  app,
}: {
  item: TimelineItem
  app: string
}) {
  const { t } = useTranslation('delivery')
  const look = statusLook(item.status)
  return (
    <li
      className="group relative flex gap-3 pb-4 last:pb-0"
      data-testid="delivery-item"
      data-status={item.status}
    >
      <span
        className="absolute top-6 bottom-0 left-2 w-px bg-border group-last:hidden"
        aria-hidden="true"
      />
      <span className="relative mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full bg-background">
        <StatusIcon status={item.status} />
      </span>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
          <p className="min-w-0 text-sm font-medium break-words text-foreground">
            {item.title}
          </p>
          <Badge variant="muted">{t(`category.${categoryOf(item)}`)}</Badge>
          <span className={`text-xs font-medium ${TONE[look.tone].text}`}>
            {t(`status.${statusKey(item.status)}`)}
          </span>
          <time
            dateTime={item.at}
            title={formatAbsoluteTime(item.at)}
            className="ml-auto text-xs text-muted-foreground tabular-nums"
          >
            {formatRelativeTime(item.at)}
          </time>
        </div>
        <Extras item={item} app={app} />
      </div>
    </li>
  )
}
