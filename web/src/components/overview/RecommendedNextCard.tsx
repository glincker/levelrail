import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { XIcon } from '@phosphor-icons/react/dist/ssr'
import {
  computeRecommendations,
  type Recommendation,
  readDismissed,
  writeDismissed,
} from '@/lib/appRecommendations'
import { Button } from '@/components/ui/button'
import { useBackupCoverage } from '../../queries/backupCoverage'
import { useGitSource } from '../../queries/gitSources'
import type { AppDetail } from '../../types/appDetail'
import type { DeployAttempt } from '../../types/deployAttempt'

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
}: {
  app: AppDetail
  latest?: DeployAttempt
}) {
  const { t } = useTranslation('common')
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

  const items = computeRecommendations({
    appName: app.name,
    hasDomain: (app.domains?.length ?? 0) > 0,
    hasHealthCheck: Boolean(app.health?.readiness ?? app.health?.liveness),
    latestDeploy: latest ? { id: latest.id, status: latest.status } : undefined,
    databases,
    hasGitSource: git.isSuccess ? true : undefined,
    previewsEnabled: git.data?.preview_enabled,
    dismissed,
  })
  if (items.length === 0) return null

  const dismiss = (id: string) => {
    const next = new Set(dismissed).add(id)
    setDismissed(next)
    writeDismissed(app.name, next)
  }

  return (
    <section
      aria-labelledby="recommended-next-title"
      className="space-y-2 rounded-xl border border-border bg-card p-4"
    >
      <h2 id="recommended-next-title" className="text-sm font-medium">
        {t('recommended.title')}
      </h2>
      <ul className="divide-y divide-border">
        {items.map((r) => {
          const title = t(`recommended.${r.id}.title`)
          return (
            <li
              key={r.id}
              className="flex flex-wrap items-center justify-between gap-2 py-2"
            >
              <span className="min-w-0 text-sm">{title}</span>
              <span className="flex items-center gap-1">
                <Button
                  size="sm"
                  variant="outline"
                  nativeButton={false}
                  render={target(r)}
                >
                  {t(`recommended.${r.id}.button`)}
                </Button>
                <Button
                  size="icon-sm"
                  variant="ghost"
                  aria-label={t('recommended.dismiss', { title })}
                  onClick={() => {
                    dismiss(r.id)
                  }}
                >
                  <XIcon aria-hidden="true" />
                </Button>
              </span>
            </li>
          )
        })}
      </ul>
    </section>
  )
}
