import { useTranslation } from 'react-i18next'
import { GitBranchGlassIcon } from '@thesvg/ui-react'
import { RelativeTime } from '@/components/kit'
import { Badge } from '@/components/ui/badge'
import { useGitSource } from '../../queries/gitSources'
import type { AppDetail } from '../../types/appDetail'
import type { DeployAttempt } from '../../types/deployAttempt'
import { AppHeaderBadges } from '../apps/AppHeaderBadges'
import { repoLabel } from '../pages/overviewHelpers'
import { dotClass, stateKey } from './deployState'
import { AutoUrlInfo } from './AutoUrlInfo'
import { TagChips } from './TagChips'
import { UrlLine } from './UrlLine'

/** One slim line: where the app lives and what its latest deploy was; Visit and Deploy stay in the header. */
export function AppSummaryStrip({
  app,
  attempts,
  url,
  attentionCount,
}: Readonly<{
  app: AppDetail
  attempts: DeployAttempt[]
  url: string | null
  attentionCount: number
}>) {
  const { t } = useTranslation('appOverview')
  const git = useGitSource(app.name).data
  const latest = attempts.find((a) => a.live) ?? attempts[0]
  const who = latest?.author ?? latest?.source
  return (
    <section
      aria-label={t('summary.label')}
      className="flex flex-wrap items-center gap-x-3 gap-y-1.5 rounded-lg border border-border bg-card px-3 py-2 text-sm"
    >
      <div className="flex min-w-0 items-center gap-0.5">
        <UrlLine appName={app.name} url={url} />
        <AutoUrlInfo appName={app.name} />
      </div>
      <p className="min-w-0 flex-1 basis-48 truncate text-xs text-muted-foreground">
        {latest?.commit_message?.split('\n')[0] ??
          (latest ? t('summary.noMessage') : t('summary.none'))}
      </p>
      {latest ? (
        <span className="inline-flex items-center gap-1.5 text-xs">
          <span
            aria-hidden="true"
            className={`size-2 shrink-0 rounded-full ${dotClass(latest.status)}`}
          />
          {t(`state.${stateKey(latest.status)}`)}
        </span>
      ) : null}
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
      {latest ? (
        <span className="text-xs text-muted-foreground tabular-nums">
          <RelativeTime at={latest.finished_at ?? latest.started_at} live />
        </span>
      ) : null}
      <TagChips appName={app.name} tags={app.tags} />
      <AppHeaderBadges
        app={app}
        attentionCount={attentionCount}
        isStatic={false}
      />
    </section>
  )
}
