import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { MoonIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { InfoTip } from '@/components/kit'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { useAppSleep, useAppSleepActions } from '../queries/appSleep'

const DEFAULT_IDLE_MINUTES = 30

/** AppSleepCard stops an app that has had no requests for a while and wakes it on the next one. */
export function AppSleepCard({ appName }: { appName: string }) {
  const { t } = useTranslation('deploys')
  const { data: sleep } = useAppSleep(appName)
  const { setIdle, wake } = useAppSleepActions(appName)
  const [minutes, setMinutes] = useState(
    String(sleep.enabled ? sleep.idle_minutes : DEFAULT_IDLE_MINUTES),
  )

  function save(value: number, success: string, hold?: boolean) {
    setIdle.mutate(
      { idle_minutes: value, hold_requests: hold },
      {
        onSuccess: () => toast.add({ title: success, type: 'success' }),
        onError: (error) =>
          toast.add({
            title: t('appSleep.toast.errorTitle'),
            description: error.message,
            type: 'error',
          }),
      },
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <MoonIcon className="size-4 text-muted-foreground" />
          {t('appSleep.title')}
          <InfoTip label={t('appSleep.infoTipLabel')}>
            {t('appSleep.description')}
          </InfoTip>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex items-center justify-between gap-4">
          <p className="text-sm font-medium text-foreground">
            {t('appSleep.enabledLabel')}
          </p>
          <Switch
            checked={sleep.enabled}
            disabled={setIdle.isPending}
            aria-label={t('appSleep.switchAriaLabel')}
            onCheckedChange={(next) =>
              save(
                next ? Number(minutes) || DEFAULT_IDLE_MINUTES : 0,
                next
                  ? t('appSleep.toast.enabled')
                  : t('appSleep.toast.disabled'),
              )
            }
          />
        </div>
        <div className="flex items-end gap-2">
          <label className="space-y-1 text-xs text-muted-foreground">
            {t('appSleep.minutesLabel')}
            <Input
              type="number"
              min={5}
              max={10080}
              value={minutes}
              onChange={(e) => setMinutes(e.target.value)}
              className="w-28"
            />
          </label>
          {sleep.enabled ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={setIdle.isPending}
              onClick={() => save(Number(minutes), t('appSleep.toast.updated'))}
            >
              {t('appSleep.save')}
            </Button>
          ) : null}
        </div>
        {sleep.enabled ? (
          <div className="flex items-center justify-between gap-4 border-t border-border pt-3">
            <div>
              <p className="text-sm font-medium text-foreground">
                {t('appSleep.holdLabel')}
              </p>
              <p className="text-xs text-muted-foreground">
                {t('appSleep.holdHelp')}
              </p>
            </div>
            <Switch
              checked={sleep.hold_requests}
              disabled={setIdle.isPending}
              aria-label={t('appSleep.holdAriaLabel')}
              onCheckedChange={(next) =>
                save(sleep.idle_minutes, t('appSleep.toast.holdUpdated'), next)
              }
            />
          </div>
        ) : null}
        {sleep.sleeping ? (
          <div className="flex items-center justify-between gap-4 border-t border-border pt-3">
            <p className="text-sm text-foreground" role="status">
              {t('appSleep.asleep')}
            </p>
            <Button
              type="button"
              size="sm"
              disabled={wake.isPending}
              onClick={() => wake.mutate()}
            >
              {t('appSleep.wakeNow')}
            </Button>
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}
