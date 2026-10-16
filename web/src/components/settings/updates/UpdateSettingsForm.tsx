import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import {
  useSetUpdateSettings,
  type UpdateChannel,
  type UpdateSettings,
} from '@/queries/updates'

const CHANNELS = ['stable', 'beta', 'edge'] as const

// UpdateSettingsForm is one form across two cards so a single Save writes
// the channel and the background-check switch together, as before.
export function UpdateSettingsForm({ settings }: { settings: UpdateSettings }) {
  const { t } = useTranslation('updates')
  const setSettings = useSetUpdateSettings()
  const [channel, setChannel] = useState<UpdateChannel>(settings.channel)
  const [autoCheck, setAutoCheck] = useState(settings.auto_update_enabled)
  const dirty =
    channel !== settings.channel || autoCheck !== settings.auto_update_enabled

  return (
    <form
      className="grid gap-4 lg:grid-cols-2"
      onSubmit={(e) => {
        e.preventDefault()
        setSettings.mutate(
          { channel, auto_update_enabled: autoCheck },
          {
            onSuccess: () => {
              toast.add({ title: t('settings.saved'), type: 'success' })
            },
          },
        )
      }}
    >
      <Card>
        <CardHeader>
          <CardTitle>{t('channels.active')}</CardTitle>
          <CardDescription>{t('settings.channelDescription')}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
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
              <SelectTrigger id="update-channel" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {CHANNELS.map((c) => (
                  <SelectItem key={c} value={c}>
                    {t(`channels.${c}.label`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <FieldDescription className="max-w-prose">
              {t(`channels.${channel}.description`)}
            </FieldDescription>
          </Field>
          <dl className="flex items-center justify-between gap-3 border-t border-border pt-3 text-sm">
            <dt className="text-muted-foreground">
              {t('settings.channelActive')}
            </dt>
            <dd className="font-medium">
              {t(`channels.${settings.channel}.label`)}
            </dd>
          </dl>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('settings.backgroundTitle')}</CardTitle>
          <CardDescription>
            {t('settings.backgroundDescription')}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-3 text-sm">
            <Switch
              id="update-auto-check"
              checked={autoCheck}
              onCheckedChange={setAutoCheck}
            />
            <label htmlFor="update-auto-check">
              {t('settings.backgroundLabel')}
            </label>
          </div>
          <p className="max-w-prose text-xs text-muted-foreground">
            {t('channel.autoCheckHelp')}
          </p>
        </CardContent>
      </Card>

      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-end lg:col-span-2">
        {setSettings.isError ? (
          <p role="alert" className="text-sm text-destructive sm:mr-auto">
            {setSettings.error.message}
          </p>
        ) : dirty ? (
          <p className="text-xs text-muted-foreground sm:mr-auto">
            {t('settings.unsaved')}
          </p>
        ) : null}
        <Button
          type="submit"
          disabled={setSettings.isPending || !dirty}
          className="w-full sm:w-auto"
        >
          {t('settings.save')}
        </Button>
      </div>
    </form>
  )
}
