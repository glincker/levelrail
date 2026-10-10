import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useQueryClient, useSuspenseQuery } from '@tanstack/react-query'
import {
  ArrowSquareOutIcon,
  CheckCircleIcon,
  ArrowCircleUpIcon,
  ArrowClockwiseIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from '../../components/ui/card'
import { Button } from '../../components/ui/button'
import { Field, FieldDescription, FieldLabel } from '../../components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../../components/ui/select'
import { Switch } from '../../components/ui/switch'
import { toast } from '../../components/ui/toast'
import {
  updatesQueryOptions,
  updateSettingsQueryOptions,
  useSetUpdateSettings,
} from '../../queries/updates'
import type { UpdateChannel, UpdateStatus } from '../../queries/updates'
import { UpgradePreflight } from '../../components/settings/UpgradePreflight'
import { ReleaseHistory } from '../../components/settings/ReleaseHistory'
import { PostUpgradeVerify } from '../../components/settings/PostUpgradeVerify'
import { PageHeader } from '../../components/shell/PageHeader'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '../../components/settings/SettingsSkeletons'

export const Route = createFileRoute('/settings/updates')({
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.ensureQueryData(updatesQueryOptions()),
      queryClient.ensureQueryData(updateSettingsQueryOptions()),
    ]),
  component: UpdatesSettingsPage,
  pendingComponent: UpdatesSettingsSkeleton,
})

function UpdatesSettingsSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <SettingsHeaderSkeleton />
      <SettingsCardSkeleton rows={2} rowVariant="line" />
      <SettingsCardSkeleton rows={3} rowVariant="list" />
    </div>
  )
}

const CHANNEL_LABELS: Record<UpdateChannel, string> = {
  stable: 'Stable (recommended)',
  beta: 'Beta (pre-release)',
  edge: 'Edge (latest main, unstable)',
}

function UpdatesSettingsPage() {
  const { data: status } = useSuspenseQuery(updatesQueryOptions())
  const { data: settings } = useSuspenseQuery(updateSettingsQueryOptions())
  const { t } = useTranslation('updates')
  const queryClient = useQueryClient()
  const setSettings = useSetUpdateSettings()
  const [channel, setChannel] = useState<UpdateChannel>(settings.channel)
  const [autoUpdateEnabled, setAutoUpdateEnabled] = useState(
    settings.auto_update_enabled,
  )
  const [checking, setChecking] = useState(false)

  const dirty =
    channel !== settings.channel ||
    autoUpdateEnabled !== settings.auto_update_enabled

  return (
    <div className="space-y-6">
      <PageHeader
        title="Updates"
        description="Current version, release channel, and available releases."
      />

      <Card>
        <CardHeader>
          <CardTitle>Version</CardTitle>
          <CardDescription>
            Compared against github.com/glincker/levelrail's published releases.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between py-1.5 text-sm">
            <span className="text-foreground">Running version</span>
            <span className="font-mono text-muted-foreground">
              {status.current_version}
            </span>
          </div>
          <UpdateStatusRow status={status} />
          <div>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={checking}
              onClick={() => {
                setChecking(true)
                void queryClient
                  .invalidateQueries({
                    queryKey: updatesQueryOptions().queryKey,
                  })
                  .finally(() => {
                    setChecking(false)
                    toast.add({ title: 'Checked for updates.', type: 'info' })
                  })
              }}
            >
              <ArrowClockwiseIcon
                className={checking ? 'size-4 animate-spin' : 'size-4'}
              />
              Check now
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Release channel</CardTitle>
          <CardDescription>
            Which release stream to compare against and, if enabled, check in
            the background. This never applies an update by itself.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault()
              setSettings.mutate(
                { channel, auto_update_enabled: autoUpdateEnabled },
                {
                  onSuccess: () => {
                    toast.add({
                      title: 'Update settings saved.',
                      type: 'success',
                    })
                  },
                },
              )
            }}
          >
            <Field>
              <FieldLabel htmlFor="update-channel">Channel</FieldLabel>
              <Select
                value={channel}
                onValueChange={(v) => {
                  if (v === 'stable' || v === 'beta' || v === 'edge') {
                    setChannel(v)
                  }
                }}
              >
                <SelectTrigger id="update-channel" className="w-full sm:w-80">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(['stable', 'beta', 'edge'] as const).map((c) => (
                    <SelectItem key={c} value={c}>
                      {CHANNEL_LABELS[c]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <div className="flex items-center gap-2 text-sm">
              <Switch
                id="update-auto-check"
                checked={autoUpdateEnabled}
                onCheckedChange={setAutoUpdateEnabled}
              />
              <label htmlFor="update-auto-check">
                Check for updates in the background
              </label>
            </div>
            <FieldDescription>
              The control plane never upgrades itself either way; this only
              controls whether it periodically checks and surfaces a notice when
              a newer release is available.
            </FieldDescription>
            {setSettings.isError ? (
              <p role="alert" className="text-sm text-destructive">
                {setSettings.error.message}
              </p>
            ) : null}
            <Button type="submit" disabled={setSettings.isPending || !dirty}>
              Save
            </Button>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Upgrade preflight</CardTitle>
          <CardDescription>
            Read-only checks before you upgrade. The control plane never
            upgrades itself.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <UpgradePreflight />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('history.title')}</CardTitle>
          <CardDescription>{t('history.description')}</CardDescription>
        </CardHeader>
        <CardContent>
          <ReleaseHistory />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('verify.title')}</CardTitle>
          <CardDescription>{t('verify.description')}</CardDescription>
        </CardHeader>
        <CardContent>
          <PostUpgradeVerify />
        </CardContent>
      </Card>
    </div>
  )
}

function UpdateStatusRow({ status }: { status: UpdateStatus }) {
  if (status.latest_version === null) {
    return (
      <p className="text-sm text-muted-foreground">
        No releases published yet.
      </p>
    )
  }

  if (!status.update_available) {
    return (
      <div className="inline-flex items-center gap-1.5 text-sm text-green-700 dark:text-green-400">
        <CheckCircleIcon className="size-4" />
        You&apos;re on the latest version.
      </div>
    )
  }

  return (
    <div className="space-y-2">
      <div className="inline-flex items-center gap-1.5 text-sm text-foreground">
        <ArrowCircleUpIcon className="size-4" />A new version is available:{' '}
        {status.latest_version}
      </div>
      {status.release_url && (
        <a
          href={status.release_url}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center gap-1 text-sm text-primary hover:underline"
        >
          View release
          <ArrowSquareOutIcon className="size-3.5" />
        </a>
      )}
    </div>
  )
}
