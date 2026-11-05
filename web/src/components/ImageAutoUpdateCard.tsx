import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { RefreshCwGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { InfoTip } from '@/components/kit'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { useImageStatus } from '../queries/dockerIntegration'
import {
  useCheckImageUpdate,
  useRotateImageUpdateWebhook,
  useImageAutoUpdate,
  useSetImageAutoUpdate,
} from '../queries/imageAutoUpdate'

// Prefill for the window length; deployments can tune it at build time.
const DEFAULT_WINDOW_MINUTES =
  Number(import.meta.env.VITE_IMAGE_UPDATE_WINDOW_MINUTES) || 120

const WINDOW_PRESETS = [
  { id: 'nightly', cron: '0 3 * * *' },
  { id: 'weekends', cron: '0 3 * * 6,0' },
] as const

/** ImageAutoUpdateCard opts an app into redeploying when its image tag moves, and checks the registry on demand. */
export function ImageAutoUpdateCard({ appName }: { appName: string }) {
  const { t } = useTranslation('deploys')
  const setting = useImageAutoUpdate(appName)
  const setEnabled = useSetImageAutoUpdate(appName)
  const check = useCheckImageUpdate(appName)
  const rotate = useRotateImageUpdateWebhook(appName)
  const imageStatus = useImageStatus(appName)

  const { last_result: lastResult, last_checked_at: lastChecked } = setting.data
  const [windowOn, setWindowOn] = useState(Boolean(setting.data.window_cron))
  const [cron, setCron] = useState(setting.data.window_cron ?? '')
  const [minutes, setMinutes] = useState(
    String(setting.data.window_minutes || DEFAULT_WINDOW_MINUTES),
  )
  const [windowError, setWindowError] = useState('')

  function saveWindow(on: boolean) {
    setWindowError('')
    const trimmed = cron.trim()
    const mins = Number(minutes)
    if (on && (!trimmed || !Number.isInteger(mins) || mins < 1)) {
      setWindowError(t('imageAutoUpdate.window.invalid'))
      return
    }
    setEnabled.mutate(
      on
        ? {
            enabled: setting.data.enabled,
            window_cron: trimmed,
            window_minutes: mins,
          }
        : { enabled: setting.data.enabled, window_cron: '' },
      {
        onSuccess: () =>
          toast.add({
            title: t('imageAutoUpdate.window.saved'),
            type: 'success',
          }),
        onError: (error) => setWindowError(error.message),
      },
    )
  }

  function toggleWindow(next: boolean) {
    setWindowOn(next)
    if (!next) {
      if (setting.data.window_cron) saveWindow(false)
      else setWindowError('')
    }
  }

  const waiting = Boolean(lastResult?.includes('waiting for the update window'))

  function toggle(next: boolean) {
    setEnabled.mutate(
      { enabled: next },
      {
        onSuccess: () =>
          toast.add({
            title: next
              ? t('imageAutoUpdate.toast.enabled')
              : t('imageAutoUpdate.toast.disabled'),
            type: 'success',
          }),
        onError: (error) =>
          toast.add({
            title: t('imageAutoUpdate.toast.errorTitle'),
            description: error.message,
            type: 'error',
          }),
      },
    )
  }

  function checkNow() {
    check.mutate(undefined, {
      onSuccess: () => {
        void imageStatus.refetch()
      },
      onError: (error) =>
        toast.add({
          title: t('imageAutoUpdate.toast.checkFailed'),
          description: error.message,
          type: 'error',
        }),
    })
  }

  const webhookUrl = rotate.data
    ? `${window.location.origin}${rotate.data.path}`
    : ''
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <RefreshCwGlassIcon className="size-5 text-muted-foreground" />
          {t('imageAutoUpdate.title')}
          <InfoTip label={t('imageAutoUpdate.infoTipLabel')}>
            {t('imageAutoUpdate.description')}
          </InfoTip>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex items-center justify-between gap-4">
          <p className="text-sm font-medium text-foreground">
            {t('imageAutoUpdate.enabledLabel')}
          </p>
          <Switch
            checked={setting.data.enabled}
            onCheckedChange={toggle}
            disabled={setEnabled.isPending}
            aria-label={t('imageAutoUpdate.switchAriaLabel')}
          />
        </div>
        {imageStatus.data?.source === 'registry' &&
        (imageStatus.data.update_available ||
          imageStatus.data.arch_mismatch) ? (
          <div className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-tone-warning-border bg-tone-warning-soft px-3 py-2">
            <div className="space-y-1 text-sm text-tone-warning">
              {imageStatus.data.update_available ? (
                <p>{t('imageAutoUpdate.status.updateAvailable')}</p>
              ) : null}
              {imageStatus.data.arch_mismatch ? (
                <p>
                  {t('imageAutoUpdate.status.archMismatch', {
                    image: imageStatus.data.image_arch,
                    node: imageStatus.data.node_arch,
                  })}
                </p>
              ) : null}
            </div>
            <Button
              type="button"
              size="sm"
              onClick={checkNow}
              disabled={check.isPending}
            >
              {t('imageAutoUpdate.status.updateNow')}
            </Button>
          </div>
        ) : null}
        <div className="flex items-center justify-between gap-4">
          <p className="text-xs text-muted-foreground" role="status">
            {lastResult
              ? t('imageAutoUpdate.lastCheck', {
                  result: lastResult,
                  when: lastChecked
                    ? new Date(lastChecked).toLocaleString()
                    : '',
                })
              : t('imageAutoUpdate.neverChecked')}
          </p>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={checkNow}
            disabled={check.isPending}
          >
            {check.isPending
              ? t('imageAutoUpdate.checking')
              : t('imageAutoUpdate.checkNow')}
          </Button>
        </div>
        <div className="space-y-2 border-t border-border pt-3">
          <div className="flex items-center justify-between gap-4">
            <p className="text-sm font-medium text-foreground">
              {t('imageAutoUpdate.window.label')}
            </p>
            <Switch
              checked={windowOn}
              onCheckedChange={toggleWindow}
              disabled={setEnabled.isPending}
              aria-label={t('imageAutoUpdate.window.switchAriaLabel')}
            />
          </div>
          {windowOn ? (
            <div className="space-y-2">
              <div
                className="flex flex-wrap items-center gap-1.5"
                role="group"
                aria-label={t('imageAutoUpdate.window.presetsLabel')}
              >
                {WINDOW_PRESETS.map((preset) => (
                  <Button
                    key={preset.id}
                    type="button"
                    size="sm"
                    variant={
                      cron.trim() === preset.cron ? 'secondary' : 'outline'
                    }
                    aria-pressed={cron.trim() === preset.cron}
                    onClick={() => setCron(preset.cron)}
                  >
                    {t(`imageAutoUpdate.window.presets.${preset.id}`)}
                  </Button>
                ))}
              </div>
              <div className="flex flex-wrap items-end gap-2">
                <div className="min-w-0 flex-1 basis-40 space-y-1">
                  <label
                    htmlFor="image-update-window-cron"
                    className="text-xs text-muted-foreground"
                  >
                    {t('imageAutoUpdate.window.cronLabel')}
                  </label>
                  <Input
                    id="image-update-window-cron"
                    value={cron}
                    placeholder="0 3 * * *"
                    className="font-mono"
                    aria-invalid={windowError ? true : undefined}
                    onChange={(e) => setCron(e.target.value)}
                  />
                </div>
                <div className="w-24 space-y-1">
                  <label
                    htmlFor="image-update-window-minutes"
                    className="text-xs text-muted-foreground"
                  >
                    {t('imageAutoUpdate.window.minutesLabel')}
                  </label>
                  <Input
                    id="image-update-window-minutes"
                    type="number"
                    min={1}
                    inputMode="numeric"
                    value={minutes}
                    onChange={(e) => setMinutes(e.target.value)}
                  />
                </div>
                <Button
                  type="button"
                  size="sm"
                  onClick={() => saveWindow(true)}
                  disabled={setEnabled.isPending}
                >
                  {t('imageAutoUpdate.window.save')}
                </Button>
              </div>
              {windowError ? (
                <p className="text-xs text-destructive" role="alert">
                  {windowError}
                </p>
              ) : null}
              {waiting ? (
                <p className="text-xs text-tone-warning" role="status">
                  {t('imageAutoUpdate.window.waiting')}
                </p>
              ) : null}
            </div>
          ) : null}
        </div>
        <div className="space-y-1.5 border-t border-border pt-3">
          <div className="flex items-center justify-between gap-4">
            <p className="text-xs text-muted-foreground">
              {setting.data.has_webhook
                ? t('imageAutoUpdate.webhookSet')
                : t('imageAutoUpdate.webhookHelp')}
            </p>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => rotate.mutate()}
              disabled={rotate.isPending}
            >
              {setting.data.has_webhook
                ? t('imageAutoUpdate.webhookRotate')
                : t('imageAutoUpdate.webhookCreate')}
            </Button>
          </div>
          {webhookUrl ? (
            <div className="space-y-1">
              <Input
                readOnly
                value={webhookUrl}
                aria-label={t('imageAutoUpdate.webhookUrlLabel')}
                onFocus={(e) => e.currentTarget.select()}
              />
              <p className="text-xs text-muted-foreground">
                {t('imageAutoUpdate.webhookShownOnce')}
              </p>
              {rotate.data?.signing_secret ? (
                <div className="space-y-1">
                  <label
                    htmlFor="image-update-signing-secret"
                    className="text-xs text-muted-foreground"
                  >
                    {t('imageAutoUpdate.signingSecretLabel')}
                  </label>
                  <Input
                    id="image-update-signing-secret"
                    readOnly
                    value={rotate.data.signing_secret}
                    className="font-mono"
                    onFocus={(e) => e.currentTarget.select()}
                  />
                </div>
              ) : null}
            </div>
          ) : null}
          {setting.data.has_webhook || webhookUrl ? (
            <ul className="space-y-1 text-xs text-muted-foreground">
              <li>{t('imageAutoUpdate.providerHint.dockerhub')}</li>
              <li>{t('imageAutoUpdate.providerHint.ghcr')}</li>
              <li>{t('imageAutoUpdate.providerHint.protected')}</li>
            </ul>
          ) : null}
        </div>
      </CardContent>
    </Card>
  )
}
