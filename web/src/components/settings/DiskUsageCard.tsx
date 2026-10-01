import { HardDriveIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import { formatBytes } from '@/lib/format'
import type { SystemStatus } from '@/queries/systemStatus'

export function DiskUsageSkeleton() {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <Skeleton className="size-8 shrink-0 rounded-lg" />
          <div className="space-y-2">
            <Skeleton className="h-4 w-32" />
            <Skeleton className="h-3.5 w-56" />
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-2">
        <Skeleton className="h-2 w-full rounded-full" />
        <Skeleton className="h-3.5 w-48" />
      </CardContent>
    </Card>
  )
}

// Only rendered when the backend actually reported disk usage
// (data_dir_total_bytes/data_dir_free_bytes are omitempty on the wire:
// no WithDataDir configured, or the statfs call itself failed), so this
// never shows a fabricated 0/0 bar.
export function DiskUsageCard({ status }: { status: SystemStatus }) {
  if (!status.data_dir_total_bytes || !status.data_dir_free_bytes) {
    return null
  }
  const usedBytes = status.data_dir_total_bytes - status.data_dir_free_bytes
  const usedPercent = Math.round(
    (usedBytes / status.data_dir_total_bytes) * 100,
  )
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <HardDriveIcon className="size-4" />
          </div>
          <div>
            <CardTitle>Data directory</CardTitle>
            <CardDescription>
              Disk usage where apps, databases, and control plane state live.
            </CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-2">
        <Progress value={usedPercent} />
        <p className="text-sm text-muted-foreground">
          {formatBytes(usedBytes)} used of{' '}
          {formatBytes(status.data_dir_total_bytes)} ({usedPercent}%)
        </p>
      </CardContent>
    </Card>
  )
}
