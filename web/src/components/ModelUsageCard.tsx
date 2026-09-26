import { useMemo, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import type { ChartRow } from '../lib/metricChart'
import { useModelUsage } from '../queries/modelKeys'
import type { ModelUsagePoint } from '../types/models'
import { MetricChartCard } from './MetricChartCard'

const WINDOWS = [
  { label: '24h', hours: 24 },
  { label: '7d', hours: 168 },
] as const

const REQUEST_COLOR = '#3b82f6'
const TOKEN_COLOR = '#10b981'
const TTFT_COLOR = '#f59e0b'

function rowsOf(
  series: ModelUsagePoint[],
  primary: (p: ModelUsagePoint) => number,
  secondary?: (p: ModelUsagePoint) => number,
): ChartRow[] {
  return series.map((p) => ({
    t: new Date(p.hour).getTime(),
    primary: primary(p),
    secondary: secondary?.(p),
  }))
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-border p-2">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="font-mono text-sm font-medium">{value}</dd>
    </div>
  )
}

export function ModelUsageCard({ modelName }: { modelName: string }) {
  const [hours, setHours] = useState<number>(WINDOWS[0].hours)
  const usage = useModelUsage(modelName, hours)
  const report = usage.data
  const range = useMemo(
    () => ({
      from: report ? new Date(report.from) : new Date(),
      to: report ? new Date(report.to) : new Date(),
    }),
    [report],
  )
  const series = report?.series ?? []
  const t = report?.totals
  const errors = t ? t.status_4xx + t.status_5xx : 0

  return (
    <section aria-label="Usage" className="space-y-3">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold">Usage</h3>
        <div className="flex gap-1">
          {WINDOWS.map((w) => (
            <Button
              key={w.hours}
              size="sm"
              variant={hours === w.hours ? 'secondary' : 'ghost'}
              onClick={() => {
                setHours(w.hours)
              }}
            >
              {w.label}
            </Button>
          ))}
        </div>
      </div>
      {usage.isPending ? (
        <Skeleton className="h-24 w-full" />
      ) : usage.error ? (
        <p className="text-sm text-destructive">{usage.error.message}</p>
      ) : t ? (
        <>
          <dl className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            <Stat label="Requests" value={t.requests.toLocaleString()} />
            <Stat
              label="Tokens in / out"
              value={`${t.input_tokens.toLocaleString()} / ${t.output_tokens.toLocaleString()}`}
            />
            <Stat
              label="Errors (rate limited)"
              value={`${errors.toLocaleString()} (${t.rate_limited.toLocaleString()})`}
            />
            <Stat
              label="Avg first byte"
              value={`${String(t.avg_ttft_ms)} ms`}
            />
          </dl>
          <MetricChartCard
            title="Requests and errors"
            unit="count"
            primaryLabel="Requests"
            primaryColor={REQUEST_COLOR}
            secondaryLabel="Errors"
            secondaryColor="#ef4444"
            rows={rowsOf(
              series,
              (p) => p.requests,
              (p) => p.status_4xx + p.status_5xx,
            )}
            range={range}
            isLoading={false}
          />
          <MetricChartCard
            title="Tokens"
            unit="count"
            primaryLabel="Input"
            primaryColor={TOKEN_COLOR}
            secondaryLabel="Output"
            secondaryColor={REQUEST_COLOR}
            rows={rowsOf(
              series,
              (p) => p.input_tokens,
              (p) => p.output_tokens,
            )}
            range={range}
            isLoading={false}
          />
          <MetricChartCard
            title="Time to first byte"
            unit="seconds"
            primaryLabel="Average"
            primaryColor={TTFT_COLOR}
            rows={rowsOf(series, (p) => p.avg_ttft_ms / 1000)}
            range={range}
            isLoading={false}
          />
          <table className="w-full text-left text-xs">
            <thead className="text-muted-foreground">
              <tr>
                <th className="py-1 font-medium">Key</th>
                <th className="py-1 font-medium">Requests</th>
                <th className="py-1 font-medium">Errors</th>
                <th className="py-1 font-medium">Tokens in / out</th>
              </tr>
            </thead>
            <tbody>
              {report.keys.map((k) => (
                <tr key={k.key_id} className="border-t border-border">
                  <td className="py-1">{k.name}</td>
                  <td className="py-1 font-mono">{k.requests}</td>
                  <td className="py-1 font-mono">
                    {k.status_4xx + k.status_5xx}
                  </td>
                  <td className="py-1 font-mono">
                    {k.input_tokens} / {k.output_tokens}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <p className="text-xs text-muted-foreground">{report.note}</p>
        </>
      ) : null}
    </section>
  )
}
