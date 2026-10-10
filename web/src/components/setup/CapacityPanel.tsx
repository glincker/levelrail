import { useTranslation } from 'react-i18next'
import { formatBytes } from '../../lib/format'
import { cn } from '@/lib/utils'
import { TONE } from '../kit/tone'
import type { Tone } from '../kit/tone'
import {
  ESTIMATE_APP_BYTES,
  ESTIMATE_APP_DISK_BYTES,
  ESTIMATE_DISK_RESERVED_BYTES,
  ESTIMATE_RESERVED_BYTES,
  estimateSmallApps,
} from '../../lib/setupReadiness'
import type { Capacity } from '../../lib/setupReadiness'
import type { DoctorReport } from '../../queries/systemDoctor'

function toneOf(report: DoctorReport, code: string): Tone {
  const status = report.checks.find((c) => c.code === code)?.status
  if (status === 'ok') return 'success'
  if (status === 'warn') return 'warning'
  if (status === 'fail') return 'danger'
  return 'neutral'
}

function Readout({
  label,
  value,
  tone,
}: {
  label: string
  value: string
  tone: Tone
}) {
  return (
    <div className="relative overflow-hidden rounded-xl border border-border bg-card p-3">
      <span
        className={cn('absolute inset-x-0 top-0 h-0.5', TONE[tone].solid)}
        aria-hidden="true"
      />
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="mt-1 text-lg font-semibold tabular-nums text-foreground">
        {value}
      </dd>
    </div>
  )
}

/** CapacityPanel shows the numbers the resource checks already measured, plus an app estimate only when memory was readable. */
export function CapacityPanel({
  report,
  capacity,
}: {
  report: DoctorReport
  capacity: Capacity
}) {
  const { t } = useTranslation('setup')
  const apps = estimateSmallApps(capacity)
  const allClear = ['ram', 'cpu', 'disk_space', 'disk_io_latency'].every(
    (code) => {
      const status = report.checks.find((c) => c.code === code)?.status
      return status === undefined || status === 'ok'
    },
  )
  let estimateLine = ''
  if (apps) {
    if (apps.limitedBy === 'disk') {
      estimateLine = t('capacity.estimateLimitedDisk', { count: apps.count })
    } else if (allClear) {
      estimateLine = t('capacity.estimateComfortable', { count: apps.count })
    } else {
      estimateLine = t('capacity.estimate', { count: apps.count })
    }
  }
  const readouts: Array<{
    key: string
    label: string
    value: string
    tone: Tone
  }> = []
  if (capacity.cpuCores !== undefined) {
    readouts.push({
      key: 'cpu',
      label: t('capacity.cpu'),
      value: String(capacity.cpuCores),
      tone: toneOf(report, 'cpu'),
    })
  }
  if (capacity.ramBytes !== undefined) {
    readouts.push({
      key: 'ram',
      label: t('capacity.ram'),
      value: formatBytes(capacity.ramBytes),
      tone: toneOf(report, 'ram'),
    })
  }
  if (capacity.diskFreeBytes !== undefined) {
    readouts.push({
      key: 'disk',
      label: t('capacity.disk'),
      value: formatBytes(capacity.diskFreeBytes),
      tone: toneOf(report, 'disk_space'),
    })
  }
  if (capacity.diskWriteMs !== undefined) {
    readouts.push({
      key: 'latency',
      label: t('capacity.latency'),
      value: t('capacity.latencyValue', {
        ms: Math.round(capacity.diskWriteMs),
      }),
      tone: toneOf(report, 'disk_io_latency'),
    })
  }
  if (readouts.length === 0) return null

  return (
    <section aria-labelledby="setup-capacity-heading" className="space-y-3">
      <div>
        <h3
          id="setup-capacity-heading"
          className="text-sm font-medium text-foreground"
        >
          {t('capacity.heading')}
        </h3>
        <p className="text-xs text-muted-foreground">{t('capacity.intro')}</p>
      </div>
      <dl className="grid grid-cols-2 gap-2 lg:grid-cols-4">
        {readouts.map((r) => (
          <Readout key={r.key} label={r.label} value={r.value} tone={r.tone} />
        ))}
      </dl>
      <p className="text-sm text-foreground">
        {apps === null ? t('capacity.noEstimate') : estimateLine}
        {apps === null ? null : (
          <span className="mt-0.5 block text-xs text-muted-foreground">
            {apps.usedDisk
              ? t('capacity.assumptionDisk', {
                  app: formatBytes(ESTIMATE_APP_BYTES),
                  reserve: formatBytes(ESTIMATE_RESERVED_BYTES),
                  disk: formatBytes(ESTIMATE_APP_DISK_BYTES),
                  diskReserve: formatBytes(ESTIMATE_DISK_RESERVED_BYTES),
                })
              : t('capacity.assumption', {
                  app: formatBytes(ESTIMATE_APP_BYTES),
                  reserve: formatBytes(ESTIMATE_RESERVED_BYTES),
                })}
          </span>
        )}
      </p>
    </section>
  )
}
