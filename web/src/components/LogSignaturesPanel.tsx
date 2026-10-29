import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  BellRingGlassIcon,
  FileSearchGlassIcon,
  SparklesGlassIcon,
} from '@thesvg/ui-react'
import { Button } from './ui/button'
import { Skeleton } from './ui/skeleton'
import { TimeRangeControls } from './TimeRangeControls'
import { useLogSignatures } from '../queries/logExplorer'
import {
  DEFAULT_TIME_RANGE_KEY,
  resolveTimeRange,
  type TimeRangeKey,
} from '../lib/timeRange'
import type { LogSignature } from '../types/logExplorer'

const SPARK_W = 96
const SPARK_H = 24

function Sparkline({ buckets, label }: { buckets: number[]; label: string }) {
  const max = Math.max(1, ...buckets)
  const step = buckets.length > 1 ? SPARK_W / (buckets.length - 1) : SPARK_W
  const points = buckets
    .map(
      (v, i) =>
        `${(i * step).toFixed(1)},${(SPARK_H - (v / max) * (SPARK_H - 2) - 1).toFixed(1)}`,
    )
    .join(' ')
  return (
    <svg
      viewBox={`0 0 ${SPARK_W} ${SPARK_H}`}
      className="h-6 w-24 shrink-0 text-destructive"
      role="img"
      aria-label={label}
    >
      <polyline
        points={points}
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinejoin="round"
      />
    </svg>
  )
}

function SignatureRow({
  sig,
  onViewLines,
  onAlert,
}: {
  sig: LogSignature
  onViewLines: (sig: LogSignature) => void
  onAlert: (sig: LogSignature) => void
}) {
  const { t } = useTranslation('observability')
  return (
    <li className="rounded-lg border border-border p-3">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0 flex-1">
          <p className="font-mono text-sm break-words text-foreground">
            {sig.template}
          </p>
          <p className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
            <span className="tabular-nums">
              {t('explorer.occurrences', { count: sig.count })}
            </span>
            <span>
              {t('explorer.firstSeen', {
                time: new Date(sig.first_seen).toLocaleString(),
              })}
            </span>
            <span>
              {t('explorer.lastSeen', {
                time: new Date(sig.last_seen).toLocaleString(),
              })}
            </span>
            {sig.new ? (
              <span className="inline-flex items-center gap-1 rounded bg-tone-warning-soft px-1.5 py-0.5 font-medium text-tone-warning">
                <SparklesGlassIcon className="size-3.5" aria-hidden="true" />
                {t('explorer.newBadge')}
              </span>
            ) : null}
          </p>
        </div>
        <Sparkline
          buckets={sig.buckets}
          label={t('explorer.sparkline', { count: sig.count })}
        />
      </div>
      {sig.sample ? (
        <p className="mt-2 rounded bg-neutral-950 px-2 py-1 font-mono text-xs break-words text-neutral-300">
          {sig.sample}
        </p>
      ) : null}
      <div className="mt-2 flex flex-wrap gap-2">
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => {
            onViewLines(sig)
          }}
        >
          <FileSearchGlassIcon className="size-4" aria-hidden="true" />
          {t('explorer.viewLines')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => {
            onAlert(sig)
          }}
        >
          <BellRingGlassIcon className="size-4" aria-hidden="true" />
          {t('explorer.alertOnThis')}
        </Button>
      </div>
    </li>
  )
}

// Top error signatures for one app: similar error lines clustered with
// counts, first/last seen and a sparkline. "New" compares against the window
// of equal length just before the selected range.
export function LogSignaturesPanel({
  appName,
  onViewLines,
  onAlert,
}: {
  appName: string
  onViewLines: (sig: LogSignature, range: { from: Date; to: Date }) => void
  onAlert: (sig: LogSignature) => void
}) {
  const { t } = useTranslation('observability')
  const [rangeKey, setRangeKey] = useState<TimeRangeKey>(DEFAULT_TIME_RANGE_KEY)
  const [nonce, setNonce] = useState(0)
  const [onlyNew, setOnlyNew] = useState(false)
  const range = useMemo(
    () => resolveTimeRange(rangeKey),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [rangeKey, nonce],
  )
  const { data, isLoading, error } = useLogSignatures(appName, {
    from: range.from,
    to: range.to,
    baseline: true,
    limit: 50,
  })
  const shown = (data?.signatures ?? []).filter((s) => !onlyNew || s.new)

  return (
    <section className="rounded-lg border border-border p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight) text-foreground">
            {t('explorer.title')}
          </h2>
          <p className="mt-1 text-xs text-muted-foreground">
            {t('explorer.subtitle')}
          </p>
        </div>
        <TimeRangeControls
          rangeKey={rangeKey}
          onRangeChange={setRangeKey}
          onRefresh={() => {
            setNonce((n) => n + 1)
          }}
        >
          <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <input
              type="checkbox"
              checked={onlyNew}
              onChange={(e) => {
                setOnlyNew(e.target.checked)
              }}
            />
            {t('explorer.onlyNew', { count: data?.new_count ?? 0 })}
          </label>
        </TimeRangeControls>
      </div>

      <div className="mt-3 space-y-2">
        {isLoading ? (
          <Skeleton className="h-24 w-full" />
        ) : error ? (
          <p className="text-sm text-destructive">{error.message}</p>
        ) : shown.length === 0 ? (
          <p className="py-6 text-center text-sm text-muted-foreground">
            {onlyNew ? t('explorer.noNew') : t('explorer.empty')}
          </p>
        ) : (
          <ul className="space-y-2">
            {shown.map((sig) => (
              <SignatureRow
                key={sig.id}
                sig={sig}
                onViewLines={(s) => {
                  onViewLines(s, range)
                }}
                onAlert={onAlert}
              />
            ))}
          </ul>
        )}
        {data?.truncated ? (
          <p className="text-xs text-muted-foreground">
            {t('explorer.truncated', { rows: data.max_rows })}
          </p>
        ) : null}
      </div>
    </section>
  )
}
