import { StackIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from '@/components/ui/card'
import { formatBytes } from '@/lib/format'
import type { DockerDiskUsage } from '@/queries/systemStatus'
import { CleanUpDockerDialog } from '@/components/CleanUpDockerDialog'

// the button actually performs.
function DockerUsageRow({
  label,
  totalBytes,
  reclaimableBytes,
}: {
  label: string
  totalBytes: number
  reclaimableBytes: number
}) {
  return (
    <div className="flex items-center justify-between py-1.5 text-sm">
      <span className="text-foreground">{label}</span>
      <span className="text-muted-foreground">
        {formatBytes(totalBytes)}
        {reclaimableBytes > 0 ? (
          <span className="ml-1.5 text-xs">
            ({formatBytes(reclaimableBytes)} reclaimable)
          </span>
        ) : null}
      </span>
    </div>
  )
}

export function DockerDiskUsageCard({ usage }: { usage: DockerDiskUsage }) {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
              <StackIcon className="size-4" />
            </div>
            <div>
              <CardTitle>Docker storage</CardTitle>
              <CardDescription>
                Space claimed by images, containers, volumes, and build cache on
                this daemon.
              </CardDescription>
            </div>
          </div>
          <CleanUpDockerDialog />
        </div>
      </CardHeader>
      <CardContent className="divide-y divide-border">
        <DockerUsageRow
          label="Images"
          totalBytes={usage.images_total_bytes}
          reclaimableBytes={usage.images_reclaimable_bytes}
        />
        <DockerUsageRow
          label="Containers"
          totalBytes={usage.containers_total_bytes}
          reclaimableBytes={usage.containers_reclaimable_bytes}
        />
        <DockerUsageRow
          label="Volumes"
          totalBytes={usage.volumes_total_bytes}
          reclaimableBytes={usage.volumes_reclaimable_bytes}
        />
        <DockerUsageRow
          label="Build cache"
          totalBytes={usage.build_cache_total_bytes}
          reclaimableBytes={usage.build_cache_reclaimable_bytes}
        />
      </CardContent>
      <CardContent className="pt-0">
        <p className="text-xs text-muted-foreground">
          &ldquo;Reclaimable&rdquo; includes every currently unused resource,
          Docker&apos;s own accounting. Clean up now is more conservative: it
          only removes dangling images and anonymous volumes, never a tagged
          image kept for rollback or a named database volume, so the amount
          actually freed can be less than the reclaimable figure above.
        </p>
      </CardContent>
    </Card>
  )
}
