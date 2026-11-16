import { Progress } from '@/components/ui/progress'
import { capacityPercent } from '../../lib/buildRunnerFormat'

// The width comes from the library's own indicator, so no inline style here.
export function CapacityBar({
  used,
  total,
  label,
}: Readonly<{ used: number; total: number; label: string }>) {
  return (
    <Progress
      value={capacityPercent(used, total)}
      aria-label={label}
      className="gap-0"
    />
  )
}
