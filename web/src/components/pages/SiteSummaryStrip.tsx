import { useTranslation } from 'react-i18next'
import { ExternalLinkGlassIcon, GitBranchGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useDeployProgress } from '../../hooks/useDeployProgress'
import { formatAge } from '../../lib/format'
import { useApp } from '../../queries/apps'
import { useGitSource } from '../../queries/gitSources'
import { usePagesSiteDetail } from '../../queries/pagesSites'
import { repoLabel } from './overviewHelpers'

/** One slim line: where the site lives and what its latest production deploy was. */
export function SiteSummaryStrip({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('pagesSites')
  const { data: app } = useApp(name)
  const { attempts } = useDeployProgress(name)
  const detail = usePagesSiteDetail(name).data
  const git = useGitSource(name).data
  const domain = app.domains?.[0]
  const latest = attempts.find((a) => a.live) ?? attempts[0]
  const when =
    latest?.finished_at ??
    latest?.started_at ??
    detail?.latest_deploy?.finished_at ??
    detail?.latest_deploy?.started_at
  const who = latest?.author ?? latest?.source
  const url = domain ? `https://${domain}` : null
  return (
    <section
      aria-label={t('overview.strip.label')}
      className="flex flex-wrap items-center gap-x-3 gap-y-1.5 rounded-lg border border-border bg-card px-3 py-2 text-sm"
    >
      {url ? (
        <a
          href={url}
          target="_blank"
          rel="noreferrer"
          className="max-w-full truncate font-mono text-xs font-medium underline-offset-2 hover:underline focus-visible:underline"
        >
          {domain}
        </a>
      ) : (
        <span className="text-xs text-muted-foreground">{t('list.noUrl')}</span>
      )}
      <p className="min-w-0 flex-1 basis-48 truncate text-xs text-muted-foreground">
        {latest?.commit_message?.split('\n')[0] ??
          (latest ? t('overview.strip.noMessage') : t('overview.strip.none'))}
      </p>
      {git ? (
        <Badge variant="outline" className="max-w-48">
          <GitBranchGlassIcon aria-hidden="true" />
          <span className="truncate">
            {repoLabel(git.repo_url)}
            {git.branch ? ` : ${git.branch}` : ''}
          </span>
        </Badge>
      ) : null}
      {who ? (
        <span className="text-xs text-muted-foreground">{who}</span>
      ) : null}
      {when ? (
        <span className="text-xs text-muted-foreground tabular-nums">
          {formatAge(when)}
        </span>
      ) : null}
      {url ? (
        <Button
          size="xs"
          render={<a href={url} target="_blank" rel="noreferrer" />}
        >
          {t('detail.visit')}
          <ExternalLinkGlassIcon aria-hidden="true" />
        </Button>
      ) : null}
    </section>
  )
}
