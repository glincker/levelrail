import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import { useApp } from '../../queries/apps'
import {
  useFunctionSettings,
  useUpdateFunctionSettings,
  type FunctionSettings,
} from '../../queries/functionControls'
import { AppSleepCard } from '../AppSleepCard'
import { PortEditor } from '../PortEditor'
import { ResourceLimitsEditor } from '../ResourceLimitsEditor'

function LimitsForm({
  name,
  settings,
}: {
  name: string
  settings: FunctionSettings
}) {
  const { t } = useTranslation('functions')
  const update = useUpdateFunctionSettings(name)
  const [timeout, setTimeoutValue] = useState(
    String(settings.request_timeout_seconds),
  )
  const [concurrency, setConcurrency] = useState(
    String(settings.max_concurrency),
  )
  const { bounds } = settings

  function save() {
    update.mutate(
      {
        request_timeout_seconds: Number(timeout) || 0,
        max_concurrency: Number(concurrency) || 0,
      },
      {
        onSuccess: () =>
          toast.add({ title: t('detail.settings.saved'), type: 'success' }),
        onError: (e) =>
          toast.add({
            title: t('detail.settings.saveFailed'),
            description: e.message,
            type: 'error',
          }),
      },
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('detail.settings.limitsTitle')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">
          {t('detail.settings.limitsHelp')}
        </p>
        <div className="grid gap-4 md:grid-cols-2">
          <div className="space-y-1.5">
            <Label htmlFor="fn-timeout">{t('detail.settings.timeout')}</Label>
            <Input
              id="fn-timeout"
              type="number"
              min={0}
              max={bounds.max_timeout_seconds}
              value={timeout}
              onChange={(e) => setTimeoutValue(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              {t('detail.settings.timeoutHelp', {
                max: bounds.max_timeout_seconds,
                effective: settings.effective_timeout_seconds,
              })}
            </p>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="fn-concurrency">
              {t('detail.settings.concurrency')}
            </Label>
            <Input
              id="fn-concurrency"
              type="number"
              min={0}
              max={bounds.max_concurrency}
              value={concurrency}
              onChange={(e) => setConcurrency(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              {t('detail.settings.concurrencyHelp', {
                max: bounds.max_concurrency,
                effective: settings.effective_concurrency,
              })}
            </p>
          </div>
        </div>
        <Button type="button" disabled={update.isPending} onClick={save}>
          {t('detail.settings.save')}
        </Button>
      </CardContent>
    </Card>
  )
}

export function FunctionSettingsTab({ name }: Readonly<{ name: string }>) {
  const { data: app } = useApp(name)
  const { data: settings } = useFunctionSettings(name)
  return (
    <div className="space-y-6">
      {settings ? (
        <LimitsForm
          key={`${settings.request_timeout_seconds}-${settings.max_concurrency}`}
          name={name}
          settings={settings}
        />
      ) : null}
      <AppSleepCard appName={name} />
      <ResourceLimitsEditor app={app} />
      <PortEditor app={app} />
    </div>
  )
}
