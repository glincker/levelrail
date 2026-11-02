import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { XGlassIcon } from '@thesvg/ui-react'
import {
  computeRecommendations,
  type Recommendation,
  type RecommendationId,
  readDismissed,
  writeDismissed,
} from '@/lib/appRecommendations'
import { Button } from '@/components/ui/button'
import { useBackupCoverage } from '../../queries/backupCoverage'
import { useGitSource } from '../../queries/gitSources'
import type { AppDetail } from '../../types/appDetail'
import type { DeployAttempt } from '../../types/deployAttempt'

// Compact strip: the rest sit behind a disclosure so metrics stay in view.
const VISIBLE_LIMIT = 2

function target(r: Recommendation) {
  if (r.to === '/apps/$name/deploys/$deployId/logs') {
    return <Link to={r.to} params={r.params} />
  }
  return <Link to={r.to} params={r.params} />
}

/** The few next steps that fit this app's real state; renders nothing when none apply. */
export function RecommendedNextCard({
  app,
  latest,
  omit,
}: {
  app: AppDetail
  latest?: DeployAttempt
  /** Recommendation ids another card on the page already covers. */
  omit?: readonly RecommendationId[]
}) {
  const { t } = useTranslation('common')
  const [expanded, setExpanded] = useState(false)
  const [dismissed, setDismissed] = useState(() => readDismissed(app.name))
  const coverage = useBackupCoverage()
  const git = useGitSource(app.name)

  const attached = new Set(
    [
      app.database_attachment?.database_name,
      ...Object.values(app.database_env ?? {}).map((d) => d.database),
    ].filter((n): n is string => Boolean(n)),
  )
  const rows = coverage.data?.resources ?? []
  const databases = coverage.isSuccess
    ? [...attached].map((n) => {
        const row = rows.find((r) => r.kind === 'database' && r.name === n)
        return {
          name: n,
          hasSchedule: row ? Boolean(row.schedule) : true,
        }
      })
    : undefined

  const all = computeRecommendations({
    appName: app.name,
    hasDomain: (app.domains?.length ?? 0) > 0,
    hasHealthCheck: Boolean(app.health?.readiness ?? app.health?.liveness),
    latestDeploy: latest ? { id: latest.id, status: latest.status } : undefined,
    databases,
    hasGitSource: git.isSuccess ? true : undefined,
    previewsEnabled: git.data?.preview_enabled,
    dismissed,
  })
  const items = all.filter((r) => !omit?.includes(r.id))
  if (items.length === 0) return null
  const hidden = expanded ? 0 : Math.max(0, items.length - VISIBLE_LIMIT)
  const shown = hidden > 0 ? items.slice(0, VISIBLE_LIMIT) : items

  const dismiss = (id: string) => {
    const next = new Set(dismissed).add(id)
    setDismissed(next)
    writeDismissed(app.name, next)
  }

  return (
    <section
      aria-labelledby="recommended-next-title"
      className="rounded-lg border border-border bg-card px-3 py-1"
    >
      <h2 id="recommended-next-title" className="sr-only">
        {t('recommended.title')}
      </h2>
      <ul className="divide-y divide-border">
        {shown.map((r) => {
          const title = t(`recommended.${r.id}.title`)
          return (
            <li
              key={r.id}
              className="flex flex-wrap items-center justify-between gap-2 py-1"
            >
              <span className="min-w-0 truncate text-sm">{title}</span>
              <span className="flex items-center gap-1">
                <Button
                  size="xs"
                  variant="outline"
                  nativeButton={false}
                  render={target(r)}
                >
                  {t(`recommended.${r.id}.button`)}
                </Button>
                <Button
                  size="icon-xs"
                  variant="ghost"
                  aria-label={t('recommended.dismiss', { title })}
                  onClick={() => {
                    dismiss(r.id)
                  }}
                >
                  <XGlassIcon size="1.4em" aria-hidden="true" />
                </Button>
              </span>
            </li>
          )
        })}
      </ul>
      {hidden > 0 || expanded ? (
        <div className="flex justify-end border-t border-border py-0.5">
          <Button
            size="xs"
            variant="ghost"
            aria-expanded={expanded}
            onClick={() => {
              setExpanded(!expanded)
            }}
          >
            {expanded
              ? t('recommended.less')
              : t('recommended.more', { count: hidden })}
          </Button>
        </div>
      ) : null}
    </section>
  )
}
