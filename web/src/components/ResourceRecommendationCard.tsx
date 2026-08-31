import {
  GaugeIcon,
  TrendUpIcon,
  TrendDownIcon,
  CheckCircleIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { useResourceRecommendation } from '../queries/resourceRecommendation'
import { formatBytes, formatNanoCpus } from '../lib/format'
import { ApiError } from '../lib/apiError'
import type {
  DimensionRecommendation,
  RecommendationConfidence,
} from '../types/resourceRecommendation'
import type { VariantProps } from 'class-variance-authority'
import type { ReactNode } from 'react'

const CONFIDENCE_LABEL: Record<RecommendationConfidence, string> = {
  high: 'High confidence',
  medium: 'Medium confidence',
  low: 'Low confidence',
}

const CONFIDENCE_BADGE_VARIANT: Record<
  RecommendationConfidence,
  VariantProps<typeof badgeVariants>['variant']
> = {
  high: 'success',
  medium: 'warning',
  low: 'muted',
}

// This card is purely informational: it never writes a resource limit
// itself, only surfaces a suggestion next to ResourceLimitsEditor so the
// operator can type the value in there if they agree with it.
export function ResourceRecommendationCard({ appName }: { appName: string }) {
  const { data, isLoading, isError, error } = useResourceRecommendation(appName)

  if (isLoading) {
    return null
  }
  if (isError) {
    // 501 (telemetry not configured) is an expected, non-error state for
    // a control plane that hasn't wired up telemetry: say nothing rather
    // than showing an alarming error card for a normal configuration.
    if (error instanceof ApiError && error.status === 501) {
      return null
    }
    return (
      <Card>
        <CardContent className="pt-6">
          <p className="text-sm text-muted-foreground">
            Could not load a resource suggestion right now.
          </p>
        </CardContent>
      </Card>
    )
  }
  if (!data) {
    return null
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <GaugeIcon className="size-4" />
          Resource suggestion
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {data.oom_detected_at ? (
          <Alert variant="destructive">
            <WarningCircleIcon />
            <AlertDescription>
              This app was OOM-killed on{' '}
              {new Date(data.oom_detected_at).toLocaleString()}.{' '}
              {data.oom_excerpt}
            </AlertDescription>
          </Alert>
        ) : null}
        <DimensionSuggestion
          label="Memory"
          rec={data.memory}
          format={formatBytes}
        />
        <DimensionSuggestion label="CPU" rec={data.cpu} format={formatNanoCpus} />
        <p className="text-xs text-muted-foreground">
          Based on the last {data.lookback_window}. This is only a
          suggestion: nothing is changed automatically, apply it below if
          you agree with it.
        </p>
      </CardContent>
    </Card>
  )
}

const ACTION_ICON: Record<string, ReactNode> = {
  raise: <TrendUpIcon className="size-4" />,
  lower: <TrendDownIcon className="size-4" />,
  keep: <CheckCircleIcon className="size-4" />,
}

const ACTION_LABEL: Record<string, string> = {
  raise: 'Consider raising',
  lower: 'Consider lowering',
  keep: 'Looks appropriate',
}

function DimensionSuggestion({
  label,
  rec,
  format,
}: {
  label: string
  rec: DimensionRecommendation
  format: (value?: number | null) => string
}) {
  return (
    <div className="rounded-md border border-border p-3">
      <div className="flex items-center justify-between gap-2">
        <p className="text-sm font-medium text-foreground">{label}</p>
        <Badge variant={CONFIDENCE_BADGE_VARIANT[rec.confidence]}>
          {CONFIDENCE_LABEL[rec.confidence]}
        </Badge>
      </div>
      {rec.action ? (
        <div className="mt-2 flex items-center gap-2 text-sm text-foreground">
          {ACTION_ICON[rec.action]}
          <span>
            {ACTION_LABEL[rec.action]}: {format(rec.suggested_limit)}
          </span>
        </div>
      ) : null}
      <p className="mt-1 text-sm text-muted-foreground">{rec.reason}</p>
      {rec.sample_count > 0 ? (
        <p className="mt-2 text-xs text-muted-foreground">
          {rec.sample_count} samples · p95 {format(rec.p95_usage)} · p99{' '}
          {format(rec.p99_usage)} · current limit {format(rec.current_limit)}
        </p>
      ) : null}
    </div>
  )
}
