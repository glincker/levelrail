import { useTranslation } from 'react-i18next'
import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  TrendingDownGlassIcon,
  TrendingUpGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import {
  useDatabaseResourceRecommendation,
  useResourceRecommendation,
} from '../../queries/resourceRecommendation'
import { formatBytes, formatNanoCpus } from '../../lib/format'
import { ApiError } from '../../lib/apiError'
import type { DimensionRecommendation } from '../../types/resourceRecommendation'

const CONFIDENCE_VARIANT = {
  high: 'success',
  medium: 'warning',
  low: 'muted',
} as const

function Dimension({
  label,
  rec,
  format,
}: {
  label: string
  rec: DimensionRecommendation
  format: (v?: number | null) => string
}) {
  const { t } = useTranslation('resourceAdvisor')
  const value = format(rec.suggested_limit)
  const icon =
    rec.action === 'raise' ? (
      <TrendingUpGlassIcon className="size-4.5" />
    ) : rec.action === 'lower' ? (
      <TrendingDownGlassIcon className="size-4.5" />
    ) : (
      <CircleCheckGlassIcon className="size-4.5 text-tone-success" />
    )
  return (
    <div className="rounded-md border border-border p-3">
      <div className="flex items-center justify-between gap-2">
        <p className="text-sm font-medium text-foreground">{label}</p>
        <Badge variant={CONFIDENCE_VARIANT[rec.confidence]}>
          {t(`suggestion.confidence.${rec.confidence}`)}
        </Badge>
      </div>
      {rec.action ? (
        <p className="mt-2 flex items-center gap-2 text-sm text-foreground">
          {icon}
          {rec.action === 'keep'
            ? t('suggestion.keep')
            : t(`suggestion.${rec.action}`, { value })}
        </p>
      ) : null}
      <p className="mt-1 text-sm text-muted-foreground">{rec.reason}</p>
      {rec.sample_count > 0 ? (
        <p className="mt-2 text-xs text-muted-foreground">
          {t('suggestion.samples', {
            count: rec.sample_count,
            p95: format(rec.p95_usage),
            p99: format(rec.p99_usage),
            limit: format(rec.current_limit),
          })}
        </p>
      ) : null}
    </div>
  )
}

interface Props {
  appName?: string
  databaseName?: string
}

// The limit suggestion shown while the advisor beta is off; with it on the
// same engine's output arrives as advisor findings instead.
export function LimitSuggestion(props: Props) {
  const { t } = useTranslation('resourceAdvisor')
  const app = useResourceRecommendation(props.appName ?? '', !!props.appName)
  const db = useDatabaseResourceRecommendation(
    props.databaseName ?? '',
    !!props.databaseName,
  )
  const { data, isPending, isError, error } = props.appName ? app : db
  if (isPending) return null
  if (isError) {
    if (error instanceof ApiError && error.status === 501) return null
    return (
      <p className="text-sm text-muted-foreground">{t('suggestion.error')}</p>
    )
  }
  return (
    <div className="space-y-3">
      {data.oom_detected_at ? (
        <Alert variant="destructive">
          <CircleAlertGlassIcon className="text-tone-danger" size="1.2em" />
          <AlertDescription>
            {t('suggestion.oom', {
              date: new Date(data.oom_detected_at).toLocaleString(),
              excerpt: data.oom_excerpt ?? '',
            })}
          </AlertDescription>
        </Alert>
      ) : null}
      <div className="grid gap-3 md:grid-cols-2">
        <Dimension
          label={t('suggestion.memory')}
          rec={data.memory}
          format={formatBytes}
        />
        <Dimension
          label={t('suggestion.cpu')}
          rec={data.cpu}
          format={formatNanoCpus}
        />
      </div>
      <p className="text-xs text-muted-foreground">
        {t('suggestion.basedOn', {
          window: data.lookback_label ?? data.lookback_window,
        })}
      </p>
    </div>
  )
}
