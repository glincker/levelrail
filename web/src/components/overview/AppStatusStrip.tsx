import { useState, type ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CircleAlertGlassIcon,
  GitBranchGlassIcon,
  ActivityGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
import { RelativeTime } from '@/components/kit'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { readDismissed, writeDismissed } from '@/lib/appRecommendations'
import { useAppHealthScore } from '../../queries/healthScore'
import { useGitSource } from '../../queries/gitSources'
import type { AppDetail } from '../../types/appDetail'
import type { DeployAttempt } from '../../types/deployAttempt'
import { AppHeaderBadges } from '../apps/AppHeaderBadges'
import { repoLabel } from '../pages/overviewHelpers'
import { dotClass, stateKey } from './deployState'
import { AutoUrlInfo } from './AutoUrlInfo'
import { TagChips } from './TagChips'
import { UrlLine } from './UrlLine'
import { useUrlReachability } from './useUrlReachability'

const chipClass =
  'inline-flex min-h-7 items-center gap-1.5 rounded-md border border-border px-2 py-0.5 text-xs outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring/60 motion-reduce:transition-none'
const linkChipClass = `${chipClass} hover:bg-muted`

function Chip({ label, children }: { label: string; children: ReactNode }) {
  return (
    <span className={chipClass} title={label} role="group" aria-label={label}>
      {children}
    </span>
  )
}

/** One strip for the address and every compact status: deploy, health, warnings and quick links. */
export function AppStatusStrip({
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
  const { reach, shareUrl } = useUrlReachability(app, url)
  const score = useAppHealthScore(app.name).data
  const [dismissed, setDismissed] = useState(() => readDismissed(app.name))
  const latest = attempts.find((a) => a.live) ?? attempts[0]
  const who = latest?.author ?? latest?.source
  const hasCheck = Boolean(app.health?.readiness ?? app.health?.liveness)
  const warnings =
    score?.categories.filter((c) => c.status !== 'pass').length ?? 0
  const suggestCheck = !hasCheck && !dismissed.has('add_health_check')
  const domainCount = app.domains?.length ?? 0

  return (
    <section
      aria-label={t('summary.label')}
      className="space-y-2 rounded-lg border border-border bg-card px-3 py-2 text-sm"
    >
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <div className="flex min-w-0 items-center gap-0.5">
          <UrlLine
            appName={app.name}
            url={url}
            reach={reach}
            shareUrl={shareUrl}
          />
          <AutoUrlInfo appName={app.name} />
        </div>
        <p className="min-w-0 flex-1 basis-48 truncate text-xs text-muted-foreground">
          {latest?.commit_message?.split('\n')[0] ??
            (latest ? t('summary.noMessage') : t('summary.none'))}
        </p>
      </div>
      <ul
        aria-label={t('status.chips')}
        className="flex flex-wrap items-center gap-1.5"
      >
        {latest ? (
          <li>
            <Link
              to="/apps/$name/deploys"
              params={{ name: app.name }}
              className={linkChipClass}
              title={t('status.deploy.title')}
            >
              <span
                aria-hidden="true"
                className={`size-2 shrink-0 rounded-full ${dotClass(latest.status)}`}
              />
              <span className="sr-only">{t('status.deploy.title')}: </span>
              {t(`state.${stateKey(latest.status)}`)}
              <span className="text-muted-foreground tabular-nums">
                <RelativeTime
                  at={latest.finished_at ?? latest.started_at}
                  live
                />
              </span>
            </Link>
          </li>
        ) : null}
        <li>
          {hasCheck ? (
            <Link
              to="/apps/$name/health"
              params={{ name: app.name }}
              className={linkChipClass}
              title={t('status.health.title')}
            >
              <ActivityGlassIcon className="size-4" aria-hidden="true" />
              {t('status.health.on')}
            </Link>
          ) : (
            <Chip label={t('status.health.none')}>
              <ActivityGlassIcon
                className="size-4 text-muted-foreground"
                aria-hidden="true"
              />
              {t('status.health.none')}
              {suggestCheck ? (
                <>
                  <Link
                    to="/apps/$name/health"
                    params={{ name: app.name }}
                    className="text-primary underline underline-offset-2"
                  >
                    {t('status.health.add')}
                  </Link>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    aria-label={t('status.health.dismiss')}
                    onClick={() => {
                      const next = new Set(dismissed).add('add_health_check')
                      setDismissed(next)
                      writeDismissed(app.name, next)
                    }}
                  >
                    <XGlassIcon size="1.2em" aria-hidden="true" />
                  </Button>
                </>
              ) : null}
            </Chip>
          )}
        </li>
        {warnings > 0 ? (
          <li>
            <Link
              to="/apps/$name/health"
              params={{ name: app.name }}
              className={linkChipClass}
              title={t('status.warnings.title', { count: warnings })}
            >
              <CircleAlertGlassIcon
                className="size-4 text-tone-warning"
                aria-hidden="true"
              />
              {t('status.warnings.label', { count: warnings })}
            </Link>
          </li>
        ) : null}
        {git ? (
          <li>
            <Badge variant="outline" className="max-w-48">
              <GitBranchGlassIcon aria-hidden="true" />
              <span className="truncate">
                {repoLabel(git.repo_url)}
                {git.branch ? ` : ${git.branch}` : ''}
              </span>
            </Badge>
          </li>
        ) : null}
        {who ? <li className="text-xs text-muted-foreground">{who}</li> : null}
        <li className="contents">
          <TagChips appName={app.name} tags={app.tags} />
        </li>
        <li className="contents">
          <AppHeaderBadges
            app={app}
            attentionCount={attentionCount}
            isStatic={false}
          />
        </li>
        <li className="ml-auto flex items-center gap-1">
          <Link
            to="/apps/$name/resources"
            params={{ name: app.name }}
            className={linkChipClass}
          >
            {t('status.resources')}
          </Link>
          <Link
            to="/apps/$name/domains"
            params={{ name: app.name }}
            className={linkChipClass}
          >
            {domainCount > 0 ? t('status.domains') : t('status.addDomain')}
          </Link>
        </li>
      </ul>
    </section>
  )
}
