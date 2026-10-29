import { createFileRoute } from '@tanstack/react-router'
import {
  BookOpenGlassIcon,
  CircleCheckGlassIcon,
  CircleXGlassIcon,
  MailGlassIcon,
  SettingsGlassIcon,
} from '@thesvg/ui-react'
import { LifebuoyIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { SettingRow, SettingRows } from '@/components/ui/setting-row'
import { useBrand } from '../../hooks/useBrand'
import {
  systemStatusQueryOptions,
  useSystemStatus,
} from '../../queries/systemStatus'
import { certificatesQueryOptions } from '../../queries/certificates'
import { GeneralPageShell } from '../../components/settings/GeneralPageShell'
import { DockerCleanupFallbackCard } from '../../components/DockerCleanupFallbackCard'
import { OrphanedVolumesCard } from '../../components/OrphanedVolumesCard'
import { SecretBindingCard } from '../../components/SecretBindingCard'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '@/components/settings/SettingsSkeletons'
import {
  DiskUsageCard,
  DiskUsageSkeleton,
} from '@/components/settings/DiskUsageCard'
import { DockerDiskUsageCard } from '@/components/settings/DockerDiskUsageCard'
import { CertificatesCard } from '@/components/settings/CertificatesCard'
import { MasterKeyCard } from '@/components/settings/MasterKeyCard'
import { SettingsHealthCard } from '@/components/settings/SettingsHealthCard'
import { EncryptionKeysCard } from '@/components/settings/EncryptionKeysCard'
import { DensityCard } from '@/components/settings/DensityCard'
import { PageWidthCard } from '@/components/settings/PageWidthCard'
import { PanelHost } from '../../components/plugins/PanelHost'

// Platform info comes from the already-warm /api/v1/brand cache via
// useBrand() (primed by routes/__root.tsx's loader). Build version lives
// on its own page, Settings > Updates, not duplicated here.
export const Route = createFileRoute('/settings/general')({
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.ensureQueryData(systemStatusQueryOptions()),
      queryClient.ensureQueryData(certificatesQueryOptions()),
    ]),
  component: GeneralSettingsPage,
  pendingComponent: GeneralSettingsSkeleton,
})

// Approximates this page's own card sequence (platform info, feature
// configuration, disk usage, Docker storage, orphaned volumes,
// certificates, master key, secret binding, more settings) closely
// enough to avoid a layout jump, without pixel-matching every card:
// several of these only render once a particular backend feature is
// configured, which isn't known yet at loading time.
function GeneralSettingsSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <SettingsHeaderSkeleton />
      <SettingsCardSkeleton headerIcon rows={2} rowVariant="line" />
      <SettingsCardSkeleton rows={4} rowVariant="line" />
      <DiskUsageSkeleton />
      <SettingsCardSkeleton
        headerIcon
        headerAction
        rows={4}
        rowVariant="line"
      />
      <SettingsCardSkeleton headerIcon rows={1} rowVariant="line" />
      <SettingsCardSkeleton headerIcon rows={3} rowVariant="list" />
      <SettingsCardSkeleton headerIcon headerAction rows={0} />
      <SettingsCardSkeleton headerIcon rows={1} rowVariant="line" />
      <SettingsCardSkeleton rows={1} rowVariant="line" />
    </div>
  )
}

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
    <SettingRow title={label}>
      {configured ? (
        <span className="inline-flex items-center gap-1.5 text-sm text-green-700 dark:text-green-400">
          <CircleCheckGlassIcon className="size-5 text-tone-success" />
          {trueLabel}
        </span>
      ) : (
        <span className="inline-flex items-center gap-1.5 text-sm text-muted-foreground">
          <CircleXGlassIcon className="size-5 text-tone-danger" />
          {falseLabel}
        </span>
      )}
    </SettingRow>
  )
}

function GeneralSettingsPage() {
  const brand = useBrand()
  const { data: status } = useSystemStatus()
  const displayName = brand.ShortName || brand.Name

  return (
    <GeneralPageShell>
      <div className="space-y-(--glinr-page-gap)">
        <SettingsHealthCard />

        <Card>
          <CardHeader>
            <div className="flex items-center gap-3">
              <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
                <SettingsGlassIcon className="size-5" />
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
            {(brand.SupportURL || brand.SupportEmail || brand.DocsURL) && (
              <div className="flex flex-wrap gap-2">
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
                {brand.SupportEmail ? (
                  <Button
                    variant="outline"
                    size="sm"
                    render={<a href={`mailto:${brand.SupportEmail}`} />}
                    nativeButton={false}
                  >
                    <MailGlassIcon size="1.4em" />
                    <span>{brand.SupportEmail}</span>
                  </Button>
                ) : null}
                {brand.DocsURL ? (
                  <Button
                    variant="outline"
                    size="sm"
                    render={
                      <a
                        href={brand.DocsURL}
                        target="_blank"
                        rel="noreferrer"
                      />
                    }
                    nativeButton={false}
                  >
                    <BookOpenGlassIcon size="1.4em" />
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
          <CardContent>
            <SettingRows>
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
              {!status.docker_connected && status.docker_error ? (
                <p className="py-3 text-xs text-muted-foreground">
                  {status.docker_error}
                </p>
              ) : null}
            </SettingRows>
          </CardContent>
        </Card>

        <PageWidthCard />

        <DensityCard />

        <DiskUsageCard status={status} />

        {status.docker_disk_usage ? (
          <DockerDiskUsageCard usage={status.docker_disk_usage} />
        ) : (
          <DockerCleanupFallbackCard />
        )}

        <OrphanedVolumesCard />

        <CertificatesCard />

        {status.secrets_configured ? <MasterKeyCard /> : null}
        {status.secrets_configured ? <EncryptionKeysCard /> : null}
        {status.secrets_configured ? <SecretBindingCard /> : null}
        <PanelHost slot="settings" />
      </div>
    </GeneralPageShell>
  )
}
