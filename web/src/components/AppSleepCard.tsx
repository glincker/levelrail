import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { toast } from '@/components/ui/toast'
import { deriveSleepState } from '../lib/sleepState'
import {
  useAppSleep,
  useAppSleepActions,
  type SleepFix,
  type SleepUpdate,
} from '../queries/appSleep'
import { SleepActivityStrip } from './sleep/SleepActivityStrip'
import { SleepEligibilityList } from './sleep/SleepEligibilityList'
import { SleepSettingsForm } from './sleep/SleepSettingsForm'
import { SleepStatusHeader } from './sleep/SleepStatusHeader'

/** AppSleepCard explains whether sleep can work for an app, why not, and offers one-click fixes. */
export function AppSleepCard({ appName }: { appName: string }) {
  const { t } = useTranslation('deploys')
  const { data: sleep } = useAppSleep(appName)
  const { setIdle, wake, sleepNow, applyFix } = useAppSleepActions(appName)
  const [fixingCode, setFixingCode] = useState<string | null>(null)
  const state = deriveSleepState(sleep, wake.isPending)
  const { blockers, warnings } = sleep
  const blocked = !sleep.eligible
  const idle = sleep.settings.idle_minutes

  const fail = (error: Error) =>
    toast.add({
      title: t('appSleep.toast.errorTitle'),
      description: error.message,
      type: 'error',
    })

  function save(patch: Partial<SleepUpdate>, success: string) {
    const body: SleepUpdate = {
      idle_minutes: idle > 0 ? idle : sleep.idle_limits.default_minutes,
      ...patch,
    }
    setIdle.mutate(body, {
      onSuccess: () => toast.add({ title: success, type: 'success' }),
      onError: fail,
    })
  }

  function fix(item: SleepFix, code: string) {
    setFixingCode(code)
    applyFix.mutate(item, {
      onError: fail,
      onSettled: () => setFixingCode(null),
    })
  }

  const actions = sleep.sleeping ? (
    <Button
      type="button"
      size="sm"
      disabled={wake.isPending}
      onClick={() => wake.mutate(undefined, { onError: fail })}
    >
      {t('appSleep.wakeNow')}
    </Button>
  ) : sleep.enabled ? (
    <Button
      type="button"
      size="sm"
      variant="outline"
      disabled={sleepNow.isPending || blocked}
      onClick={() => sleepNow.mutate(undefined, { onError: fail })}
    >
      {t('appSleep.sleepNow')}
    </Button>
  ) : null

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('appSleep.title')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <SleepStatusHeader state={state} sleep={sleep} actions={actions} />
        {sleep.enabled ? (
          <>
            {blockers.length > 0 || warnings.length > 0 ? (
              <SleepEligibilityList
                blockers={blockers}
                warnings={warnings}
                pendingCode={fixingCode}
                onFix={fix}
              />
            ) : null}
            {sleep.sleeping ? null : (
              <SleepActivityStrip activity={sleep.activity} />
            )}
            <SleepSettingsForm
              sleep={sleep}
              pending={setIdle.isPending}
              onSave={save}
            />
            <Button
              type="button"
              size="sm"
              variant="ghost"
              disabled={setIdle.isPending}
              onClick={() =>
                setIdle.mutate(
                  { idle_minutes: 0 },
                  {
                    onSuccess: () =>
                      toast.add({
                        title: t('appSleep.toast.disabled'),
                        type: 'success',
                      }),
                    onError: fail,
                  },
                )
              }
            >
              {t('appSleep.turnOff')}
            </Button>
          </>
        ) : (
          <>
            <SleepEligibilityList
              blockers={blockers}
              warnings={warnings}
              pendingCode={fixingCode}
              onFix={fix}
            />
            <div className="flex flex-wrap items-center gap-3">
              <Button
                type="button"
                size="sm"
                disabled={blocked || setIdle.isPending}
                aria-describedby="sleep-turn-on-hint"
                onClick={() => save({}, t('appSleep.toast.enabled'))}
              >
                {t('appSleep.turnOn')}
              </Button>
              <p
                id="sleep-turn-on-hint"
                className="min-w-0 text-xs text-muted-foreground"
              >
                {blocked
                  ? t('appSleep.turnOnBlocked', {
                      titles: blockers.map((b) => b.title).join(', '),
                    })
                  : t('appSleep.turnOnHint', {
                      minutes: sleep.idle_limits.default_minutes,
                    })}
              </p>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}
