import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  CircleCheckGlassIcon,
  CircleDashedGlassIcon,
  CopyGlassIcon,
  ExternalLinkGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { CertTruthBadge } from '../CertTruthBadge'
import { certificatesQueryOptions } from '../../queries/certificates'
import { usePagesSiteDetail } from '../../queries/pagesSites'
import type { AppUrls } from '../../queries/appUrls'
import { SecurityChip } from './ReachParts'
import { useCopyAddress } from './useCopyAddress'
import { deriveReach, type ReachAddress } from './reachModel'

/** The live address of a static site: custom domain first, then the automatic one. */
function pickStaticAddress(urls: AppUrls): ReachAddress | null {
  const model = deriveReach(urls)
  const domains = [model.hero, ...model.others, ...model.advanced].filter(
    (a): a is ReachAddress => a !== null && a.kind === 'domain' && !!a.href,
  )
  return domains.find((d) => !d.automatic) ?? domains[0] ?? null
}

function hostOf(display: string): string {
  return display.replace(/^https?:\/\//, '').replace(/\/.*$/, '')
}

/** Static variant of the reach card: no container, port or health probe, only the served address. */
export function StaticReach({
  appName,
  urls,
}: {
  appName: string
  urls: AppUrls
}) {
  const { t } = useTranslation('appAccess')
  const { copied, copy } = useCopyAddress()
  const detail = usePagesSiteDetail(appName)
  const certs = useQuery({ ...certificatesQueryOptions(), retry: false })
  const addr = pickStaticAddress(urls)
  const published = detail.data?.published ?? false
  const primaryHost = addr ? hostOf(addr.display) : ''
  const extra = (detail.data?.domains ?? []).filter((d) => d !== primaryHost)
  const updated = detail.data?.updated_at
    ? new Date(detail.data.updated_at).toLocaleString()
    : null

  if (detail.isPending) {
    return <Skeleton className="h-10 w-full" aria-hidden="true" />
  }
  return (
    <div className="space-y-2">
      <section
        aria-label={t('static.primary')}
        className="space-y-2 rounded-lg border border-primary/20 bg-primary/5 px-3 py-2"
      >
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1.5">
          {published ? (
            <Badge variant="success" role="status">
              <CircleCheckGlassIcon size="1.4em" aria-hidden="true" />
              {t('static.published')}
            </Badge>
          ) : (
            <Badge variant="muted" role="status">
              <CircleDashedGlassIcon size="1.4em" aria-hidden="true" />
              {t('static.notPublished')}
            </Badge>
          )}
          {published && addr ? (
            <>
              <code className="min-w-0 flex-[1_1_14rem] break-all font-mono text-sm font-medium">
                {addr.display}
              </code>
              <SecurityChip security={addr.security} />
              <div className="flex shrink-0 items-center gap-1">
                <a
                  href={addr.href}
                  target="_blank"
                  rel="noreferrer noopener"
                  className={buttonVariants({ size: 'sm', variant: 'default' })}
                >
                  <ExternalLinkGlassIcon
                    className="size-5"
                    aria-hidden="true"
                  />
                  {t('static.openSite')}
                </a>
                <Button
                  size="icon-sm"
                  variant="ghost"
                  aria-label={
                    copied ? t('actions.copied') : t('actions.copyUrl')
                  }
                  onClick={() => copy(addr.value)}
                >
                  {copied ? (
                    <CheckGlassIcon className="size-5" aria-hidden="true" />
                  ) : (
                    <CopyGlassIcon className="size-5" aria-hidden="true" />
                  )}
                </Button>
              </div>
            </>
          ) : (
            <>
              <span className="min-w-0 flex-1 text-sm text-muted-foreground">
                {t('static.notPublishedHint')}
              </span>
              <Link
                to="/pages/$name"
                params={{ name: appName }}
                className={buttonVariants({ size: 'sm', variant: 'default' })}
              >
                {t('static.publish')}
              </Link>
            </>
          )}
        </div>
        {published && updated ? (
          <p className="text-xs text-muted-foreground">
            {t('static.updated', { when: updated })}
          </p>
        ) : null}
      </section>
      <section aria-label={t('static.domains')}>
        <div className="flex items-center justify-between gap-2">
          <h3 className="text-xs font-medium text-muted-foreground">
            {t('static.domains')}
          </h3>
          <Link
            to="/pages/$name"
            params={{ name: appName }}
            search={{ tab: 'domains' }}
            className="text-xs text-primary underline underline-offset-2"
          >
            {t('static.addDomain')}
          </Link>
        </div>
        {extra.length > 0 ? (
          <ul className="divide-y divide-border">
            {extra.map((d) => (
              <li
                key={d}
                className="flex flex-wrap items-center justify-between gap-2 py-1.5"
              >
                <code className="min-w-0 break-all font-mono text-sm">{d}</code>
                <CertTruthBadge
                  cert={certs.data?.find((c) => c.domain === d)}
                  showRenewal={false}
                />
              </li>
            ))}
          </ul>
        ) : null}
      </section>
    </div>
  )
}
