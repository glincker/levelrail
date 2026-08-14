import { createFileRoute } from '@tanstack/react-router'
import {
  BookOpenIcon,
  CircleCheckIcon,
  CircleXIcon,
  HardDriveIcon,
  LifeBuoyIcon,
  SettingsIcon,
  SparklesIcon,
} from 'lucide-react'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Progress } from '@/components/ui/progress'
import { useBrand } from '../../hooks/useBrand'
import { formatBytes } from '../../lib/format'
import {
  systemStatusQueryOptions,
  useSystemStatus,
} from '../../queries/systemStatus'
import type { SystemStatus } from '../../queries/systemStatus'

// Real content for the General settings page. Platform info comes from
// the already-warm /api/v1/brand cache via useBrand() (primed by
// routes/__root.tsx's loader). The status card below is new: GET
// /api/v1/system/status (internal/api/status.go) closes the exact gap
// this file's own prior comment named ("no system-status card here...
// none of that exists as a backend endpoint yet"). Still deliberately
// missing from that endpoint, and so still absent here: Docker
// connectivity and a version string, neither exists on the backend yet
// either (see status.go's own doc comment for why), not faked here.
export const Route = createFileRoute('/settings/general')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(systemStatusQueryOptions()),
  component: GeneralSettingsPage,
})

function ConfiguredRow({
  label,
  configured,
}: {
  label: string
  configured: boolean
}) {
  return (
    <div className="flex items-center justify-between py-1.5 text-sm">
      <span className="text-foreground">{label}</span>
      {configured ? (
        <span className="inline-flex items-center gap-1.5 text-green-700 dark:text-green-400">
          <CircleCheckIcon className="size-4" />
          Configured
        </span>
      ) : (
        <span className="inline-flex items-center gap-1.5 text-muted-foreground">
          <CircleXIcon className="size-4" />
          Not configured
        </span>
      )}
    </div>
  )
}

// Only rendered when the backend actually reported disk usage
// (data_dir_total_bytes/data_dir_free_bytes are omitempty on the wire:
// no WithDataDir configured, or the statfs call itself failed), so this
// never shows a fabricated 0/0 bar.
function DiskUsageCard({ status }: { status: SystemStatus }) {
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

function GeneralSettingsPage() {
  const brand = useBrand()
  const { data: status } = useSystemStatus()
  const displayName = brand.ShortName || brand.Name

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-semibold text-foreground">General</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          System status and configuration.
        </p>
      </div>

      <Card>
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
              <SettingsIcon className="size-4" />
            </div>
            <div>
              <CardTitle>{displayName}</CardTitle>
              <CardDescription>Platform information</CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-sm text-muted-foreground">
            You&apos;re running {displayName}, a self-hosted deployment
            platform: push to a git repo and get a running app back, with TLS,
            logs, metrics, and rollback handled for you. This instance and
            everything it manages runs on your own infrastructure.
          </p>
          {(brand.SupportURL || brand.DocsURL) && (
            <div className="flex flex-wrap gap-2 pt-1">
              {brand.SupportURL ? (
                <Button
                  variant="outline"
                  size="sm"
                  render={
                    <a
                      href={brand.SupportURL}
                      target="_blank"
                      rel="noreferrer"
                    />
                  }
                >
                  <LifeBuoyIcon />
                  <span>Support</span>
                </Button>
              ) : null}
              {brand.DocsURL ? (
                <Button
                  variant="outline"
                  size="sm"
                  render={
                    <a href={brand.DocsURL} target="_blank" rel="noreferrer" />
                  }
                >
                  <BookOpenIcon />
                  <span>Documentation</span>
                </Button>
              ) : null}
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Feature configuration</CardTitle>
          <CardDescription>
            Optional backend features and whether this instance has them wired
            up.
          </CardDescription>
        </CardHeader>
        <CardContent className="divide-y divide-border">
          <ConfiguredRow
            label="Secrets (encrypted env vars)"
            configured={status.secrets_configured}
          />
          <ConfiguredRow
            label="Metrics and logs"
            configured={status.telemetry_configured}
          />
          <ConfiguredRow
            label="Alert rules"
            configured={status.alerts_configured}
          />
        </CardContent>
      </Card>

      <DiskUsageCard status={status} />

      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <CardTitle>More settings, coming later</CardTitle>
            <Badge variant="muted">Planned</Badge>
          </div>
          <CardDescription>
            Docker connectivity and a build version aren&apos;t wired up on the
            backend yet. Notification channels live on individual alert rules
            for now, not as a global setting here.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex items-start gap-2 text-sm text-muted-foreground">
            <SparklesIcon className="mt-0.5 size-4 shrink-0" />
            <span>
              The feature configuration above reflects this instance&apos;s real
              backend state, not a placeholder.
            </span>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
