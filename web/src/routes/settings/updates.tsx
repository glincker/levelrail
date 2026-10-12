import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useQueryClient, useSuspenseQuery } from '@tanstack/react-query'
import {
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
import type { UpdateChannel } from '../../queries/updates'
import { UpgradePreflight } from '../../components/settings/UpgradePreflight'
import { UpgradeAttempts } from '../../components/settings/UpgradeAttempts'
import { UpgradeNow } from '../../components/settings/UpgradeNow'
import { AutomaticUpgrades } from '../../components/settings/AutomaticUpgrades'
import { UpdateStatusRow } from '../../components/settings/UpdateStatusRow'
import { ServerReadiness } from '../../components/settings/ServerReadiness'
import { ReleaseHistory } from '../../components/settings/ReleaseHistory'
import { UpgradeHistory } from '../../components/settings/UpgradeHistory'
import { PostUpgradeVerify } from '../../components/settings/PostUpgradeVerify'
import { PageTemplate } from '@/components/shell/PageTemplate'
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
    <PageTemplate
      icon={<ArrowCircleUpIcon />}
      title={t('page.title')}
      description={t('page.description')}
    >
      <Card>
        <CardHeader>
          <CardTitle>Version</CardTitle>
          <CardDescription>
            Compared against github.com/glincker/levelrail's published releases.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between py-1.5 text-sm">
            <span className="text-foreground">{t('version.running')}</span>
            <span className="font-mono text-muted-foreground">
              {status.current_version}
            </span>
          </div>
          <div className="space-y-0.5 py-1.5 text-sm">
            <div className="flex items-center justify-between">
              <span className="text-foreground">{t('channels.active')}</span>
              <span className="text-muted-foreground">
                {t(`channels.${settings.channel}.label`)}
              </span>
            </div>
            <p className="text-xs text-muted-foreground">
              {t(`channels.${settings.channel}.description`)}
            </p>
          </div>
          <UpdateStatusRow status={status} />
          <UpgradeNow />
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
          <CardDescription>{t('channel.description')}</CardDescription>
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
              <FieldLabel htmlFor="update-channel">
                {t('channels.label')}
              </FieldLabel>
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
                      {t(`channels.${c}.label`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <FieldDescription>
                {t(`channels.${channel}.description`)}
              </FieldDescription>
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
            <FieldDescription>{t('channel.autoCheckHelp')}</FieldDescription>
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
          <CardTitle>{t('auto.title')}</CardTitle>
          <CardDescription>{t('auto.description')}</CardDescription>
        </CardHeader>
        <CardContent>
          <AutomaticUpgrades />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('preflight.cardTitle')}</CardTitle>
          <CardDescription>{t('preflight.cardDescription')}</CardDescription>
        </CardHeader>
        <CardContent>
          <UpgradePreflight hideEdgeGuidance />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('selfUpgrade.attemptsTitle')}</CardTitle>
          <CardDescription>{t('selfUpgrade.description')}</CardDescription>
        </CardHeader>
        <CardContent>
          <UpgradeAttempts />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('readiness.title')}</CardTitle>
          <CardDescription>{t('readiness.description')}</CardDescription>
        </CardHeader>
        <CardContent>
          <ServerReadiness />
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
          <CardTitle>{t('upgradeHistory.title')}</CardTitle>
          <CardDescription>{t('upgradeHistory.description')}</CardDescription>
        </CardHeader>
        <CardContent>
          <UpgradeHistory />
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
    </PageTemplate>
  )
}
