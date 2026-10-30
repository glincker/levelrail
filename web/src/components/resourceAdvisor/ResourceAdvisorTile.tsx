import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { LightbulbGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import {
  useAdvisorFindings,
  useAdvisorSummary,
  useResourceAdvisorEnabled,
} from '../../queries/resourceAdvisor'
import { splitPending } from '../../lib/resourceAdvisor'
import { formatBytes } from '../../lib/format'

// Dashboard summary that links into the advisor; renders nothing while the
// beta is off.
export function ResourceAdvisorTile() {
  const { t } = useTranslation('resourceAdvisor')
  const enabled = useResourceAdvisorEnabled()
  const { data } = useAdvisorSummary()
  if (!enabled || !data) return null
  const top = data.top_savings[0]
  return (
    <Card data-testid="advisor-tile">
      <CardContent className="flex flex-wrap items-center justify-between gap-3 pt-6">
        <div className="flex min-w-0 items-start gap-3">
          <LightbulbGlassIcon className="mt-0.5 size-6 shrink-0 text-muted-foreground" />
          <div className="min-w-0 space-y-1">
            <p className="flex items-center gap-2 text-sm font-medium text-foreground">
              {t('tile.title')}
              <Badge variant="outline" title={t('beta.hint')}>
                {t('beta.label')}
              </Badge>
            </p>
            <p className="text-sm text-muted-foreground">
              {data.total > 0
                ? t('tile.count', { count: data.total })
                : t('tile.none')}
              {data.saving_memory_bytes > 0
                ? `. ${t('summary.saving', { amount: formatBytes(data.saving_memory_bytes) })}`
                : ''}
            </p>
            {top ? (
              <p className="truncate text-xs text-muted-foreground">
                {top.title}
              </p>
            ) : null}
          </div>
        </div>
        <Link
          to="/resource-advisor"
          className="text-sm font-medium text-foreground underline-offset-4 hover:underline"
        >
          {t('tile.open')}
        </Link>
      </CardContent>
    </Card>
  )
}

// One-line pointer on a resource overview to its Optimise panel.
export function OptimiseHint({
  kind,
  name,
}: {
  kind: 'app' | 'database'
  name: string
}) {
  const { t } = useTranslation('resourceAdvisor')
  const { data } = useAdvisorFindings(`${kind}:${name}`)
  const count = data ? splitPending(data.findings).advice.length : 0
  if (count === 0) return null
  const cls =
    'text-sm font-medium text-foreground underline-offset-4 hover:underline'
  return (
    <p
      className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground"
      data-testid="optimise-hint"
    >
      <LightbulbGlassIcon className="size-5" />
      {t('hint.count', { count, name })}
      {kind === 'app' ? (
        <Link to="/apps/$name/resources" params={{ name }} className={cls}>
          {t('hint.open')}
        </Link>
      ) : (
        <Link to="/databases/$name/resources" params={{ name }} className={cls}>
          {t('hint.open')}
        </Link>
      )}
    </p>
  )
}
