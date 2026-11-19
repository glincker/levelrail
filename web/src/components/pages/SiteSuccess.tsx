import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { CircleCheckGlassIcon, CopyGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import { appDetailQueryOptions } from '../../queries/apps'

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

/** Shown in place of the form once a publish lands: URL chip and three next steps, nothing to confirm. */
export function SiteSuccess({
  name,
  onNavigate,
}: Readonly<{ name: string; onNavigate?: () => void }>) {
  const { t } = useTranslation('pagesSites')
  const next = [
    { tab: 'domains', label: t('success.addDomain') },
    { tab: 'routes', label: t('success.editRoutes') },
    { tab: 'deployments', label: t('success.viewDeploys') },
  ] as const
  return (
    <div className="space-y-3" role="status">
      <p className="flex items-center gap-1.5 text-sm font-medium">
        <CircleCheckGlassIcon
          className="size-4 text-success"
          aria-hidden="true"
        />
        {t('success.title', { name })}
      </p>
      <SiteUrlChip name={name} />
      <div className="flex flex-wrap items-center gap-2 text-xs">
        <span className="text-muted-foreground">{t('success.next')}</span>
        {next.map((n) => (
          <Button
            key={n.tab}
            size="xs"
            variant="outline"
            onClick={onNavigate}
            render={
              <Link
                to="/pages/$name"
                params={{ name }}
                search={n.tab === 'deployments' ? {} : { tab: n.tab }}
              />
            }
          >
            {n.label}
          </Button>
        ))}
      </div>
    </div>
  )
}
