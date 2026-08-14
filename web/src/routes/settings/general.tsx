import { createFileRoute } from '@tanstack/react-router'
import {
  BookOpenIcon,
  CheckCircleIcon,
  WarningCircleIcon,
  XCircleIcon,
  HardDriveIcon,
  LifebuoyIcon,
  GearIcon,
  ShieldCheckIcon,
  SparkleIcon,
} from '@phosphor-icons/react/dist/ssr'
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
import {
  certificatesQueryOptions,
  useCertificates,
} from '../../queries/certificates'
import type { CertificateStatus } from '../../queries/certificates'

// Real content for the General settings page. Platform info comes from
// the already-warm /api/v1/brand cache via useBrand() (primed by
// routes/__root.tsx's loader). The status card below closes the gap
// this file's own prior comment named ("no system-status card here...
// none of that exists as a backend endpoint yet"), including Docker
// daemon connectivity. Still deliberately missing: a build version
// string, no build-time version variable exists anywhere in this
// codebase yet, not faked here.
export const Route = createFileRoute('/settings/general')({
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.ensureQueryData(systemStatusQueryOptions()),
      queryClient.ensureQueryData(certificatesQueryOptions()),
    ]),
  component: GeneralSettingsPage,
})

function ConfiguredRow({
  label,
  configured,
  trueLabel = 'Configured',
  falseLabel = 'Not configured',
}: {
  label: string
  configured: boolean
  trueLabel?: string
  falseLabel?: string
}) {
  return (
    <div className="flex items-center justify-between py-1.5 text-sm">
      <span className="text-foreground">{label}</span>
      {configured ? (
        <span className="inline-flex items-center gap-1.5 text-green-700 dark:text-green-400">
          <CheckCircleIcon className="size-4" />
          {trueLabel}
        </span>
      ) : (
        <span className="inline-flex items-center gap-1.5 text-muted-foreground">
          <XCircleIcon className="size-4" />
          {falseLabel}
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

// certStatusMeta maps internal/api/certificates.go's three Status
// values to a Badge variant and label. No fourth "renewal failed" state:
// this codebase's TLS automation only drives Caddy's offline internal
// issuer today (real ACME is a separate, still-open gap), so expiry is
// the one honest signal the backend actually has, see
// queries/certificates.ts's own CertificateStatus doc comment.
const certStatusMeta: Record<
  CertificateStatus['status'],
  { label: string; variant: 'success' | 'warning' | 'destructive' }
> = {
  healthy: { label: 'Healthy', variant: 'success' },
  expiring_soon: { label: 'Expiring soon', variant: 'warning' },
  expired: { label: 'Expired', variant: 'destructive' },
}

function CertificateRow({ cert }: { cert: CertificateStatus }) {
  // Deliberately no "N days left" countdown here: computing that needs
  // Date.now() at render time, which react-hooks/purity flags as an
  // impure render call (the "now" it captures would silently go stale
  // between renders anyway). The absolute date plus the badge's
  // healthy/expiring_soon/expired state already carries the same
  // information without it.
  const notAfter = new Date(cert.not_after)
  const meta = certStatusMeta[cert.status]
  const StatusIcon =
    cert.status === 'healthy'
      ? CheckCircleIcon
      : cert.status === 'expiring_soon'
        ? WarningCircleIcon
        : XCircleIcon

  return (
    <div className="flex items-center justify-between gap-3 py-2 text-sm">
      <div className="min-w-0">
        <p className="truncate font-medium text-foreground">{cert.domain}</p>
        <p className="text-xs text-muted-foreground">
          {cert.status === 'expired' ? 'Expired' : 'Expires'}{' '}
          {notAfter.toLocaleDateString()}
        </p>
      </div>
      <Badge variant={meta.variant}>
        <StatusIcon />
        {meta.label}
      </Badge>
    </div>
  )
}

// CertificatesCard closes the gap this file's own prior comment left
// open: this route used to mention TLS only in the marketing paragraph
// above, with zero live status anywhere. CLAUDE.md section 10 names "a
// cert renewal fails silently at 3am" as this project's central risk;
// this card is the read-only surface that makes an at-risk certificate
// visible before that happens, not a management UI: renewal is
// automatic (see internal/ingress), so there is nothing to click here.
function CertificatesCard() {
  const { data: certificates } = useCertificates()

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <ShieldCheckIcon className="size-4" />
          </div>
          <div>
            <CardTitle>TLS certificates</CardTitle>
            <CardDescription>
              Automatic HTTPS certificate status per domain.
            </CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent
        className={certificates.length > 0 ? 'divide-y divide-border' : ''}
      >
        {certificates.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No certificates issued yet.
          </p>
        ) : (
          certificates.map((cert) => (
            <CertificateRow key={cert.domain} cert={cert} />
          ))
        )}
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
              <GearIcon className="size-4" />
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
                  nativeButton={false}
                >
                  <LifebuoyIcon />
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
                  nativeButton={false}
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
          <ConfiguredRow
            label="Docker daemon"
            configured={status.docker_connected}
            trueLabel="Connected"
            falseLabel="Not connected"
          />
        </CardContent>
      </Card>

      <DiskUsageCard status={status} />

      <CertificatesCard />

      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <CardTitle>More settings, coming later</CardTitle>
            <Badge variant="muted">Planned</Badge>
          </div>
          <CardDescription>
            A build version isn&apos;t wired up on the backend yet. Notification
            channels live on individual alert rules for now, not as a global
            setting here.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex items-start gap-2 text-sm text-muted-foreground">
            <SparkleIcon className="mt-0.5 size-4 shrink-0" />
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
