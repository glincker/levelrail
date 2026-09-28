import { CpuIcon, MemoryIcon } from '@phosphor-icons/react/dist/ssr'
import { InfoTip, SkeletonTile, Sparkline } from '@/components/kit'
import { formatBytes } from '../../lib/format'
import { usageRatio, type ResourceReading } from './useResourceReading'

function Bar({ ratio, label }: { ratio: number; label: string }) {
  const high = ratio >= 0.9
  return (
    <div
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={Math.round(ratio * 100)}
      className="mt-2 h-1 overflow-hidden rounded-full bg-muted"
    >
      <div
        className={`h-full rounded-full transition-[width] duration-200 ease-out motion-reduce:transition-none ${high ? 'bg-destructive' : 'bg-primary'}`}
        style={{ width: `${ratio * 100}%` }}
      />
    </div>
  )
}

export function ResourceTile({
  label,
  icon,
  value,
  context,
  ratio,
  series,
  info,
}: {
  label: string
  icon: React.ReactNode
  value: string
  context: string
  ratio: number
  series: number[]
  info: string
}) {
  return (
    <div className="rounded-xl border border-border bg-card p-3">
      <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
        {icon}
        <span>{label}</span>
        <InfoTip label={`About ${label}`}>{info}</InfoTip>
      </div>
      <p className="mt-1 text-2xl font-semibold tabular-nums">{value}</p>
      <p className="text-xs text-muted-foreground">{context}</p>
      {series.length > 1 ? (
        <Sparkline
          values={series}
          tone="neutral"
          width="fill"
          height={20}
          ariaLabel={`${label} trend`}
        />
      ) : null}
      <Bar ratio={ratio} label={`${label} usage`} />
    </div>
  )
}

export function ResourceTiles({ reading }: { reading: ResourceReading }) {
  if (reading.isPending) {
    return (
      <>
        <SkeletonTile />
        <SkeletonTile />
      </>
    )
  }
  if (reading.isError) return null
  const { cpuNow, memNow, memLimit, cores } = reading
  const cpuRatio = usageRatio(cpuNow, cores > 0 ? cores * 100 : 100)

  return (
    <>
      <ResourceTile
        label="CPU"
        icon={<CpuIcon className="size-3.5" />}
        value={cpuNow === null ? 'n/a' : `${cpuNow.toFixed(1)}%`}
        context={cores > 0 ? `of ${cores} CPU limit` : 'no CPU limit set'}
        ratio={cpuRatio}
        series={reading.cpuSeries}
        info="Container CPU use over the last 15 minutes."
      />
      <ResourceTile
        label="Memory"
        icon={<MemoryIcon className="size-3.5" />}
        value={memNow === null ? 'n/a' : formatBytes(memNow)}
        context={
          memLimit > 0 ? `of ${formatBytes(memLimit)}` : 'no memory limit set'
        }
        ratio={usageRatio(memNow, memLimit)}
        series={reading.memSeries}
        info="Container memory in use, against the configured limit when one is set."
      />
    </>
  )
}
