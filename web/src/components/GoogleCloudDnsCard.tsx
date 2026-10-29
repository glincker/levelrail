import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { CloudGlassIcon } from '@thesvg/ui-react'
import {
  useDisconnectGoogleCloudDns,
  useGoogleCloudDnsSettings,
  useTestGoogleCloudDns,
  useUpdateGoogleCloudDns,
} from '../queries/googleCloudDns'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import { Switch } from '@/components/ui/switch'
import { SettingsFormAlerts } from './SettingsCard'

// Instance-level Google Cloud DNS connection. Cloudflare and Route53 win
// when several providers are enabled. The key is write-only: only the
// project id and client email come back.
export function GoogleCloudDnsCard() {
  const { t } = useTranslation('domains')
  const { data: settings } = useGoogleCloudDnsSettings()
  const update = useUpdateGoogleCloudDns()
  const disconnect = useDisconnectGoogleCloudDns()
  const test = useTestGoogleCloudDns()

  const [enabled, setEnabled] = useState<boolean | null>(null)
  const [keyJson, setKeyJson] = useState('')
  const [formError, setFormError] = useState<string | null>(null)

  if (!settings) return null
  const isEnabled = enabled ?? settings.enabled
  const pending = update.isPending || disconnect.isPending

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setFormError(null)
    if (isEnabled && !settings?.has_key && !keyJson.trim()) {
      setFormError(t('googleCloudDns.keyRequired'))
      return
    }
    update.mutate(
      {
        enabled: isEnabled,
        service_account_json: keyJson.trim() || undefined,
      },
      {
        onSuccess: () => {
          setKeyJson('')
          setEnabled(null)
          toast.add({ title: t('googleCloudDns.saved'), type: 'success' })
        },
      },
    )
  }

  function handleDisconnect() {
    disconnect.mutate(undefined, {
      onSuccess: () => {
        setKeyJson('')
        setEnabled(null)
        test.reset()
        toast.add({ title: t('googleCloudDns.disconnected'), type: 'success' })
      },
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <CloudGlassIcon className="size-4.5" />
          {t('googleCloudDns.title')}
        </CardTitle>
        <CardDescription>{t('googleCloudDns.description')}</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} noValidate className="space-y-5">
          <div className="flex items-center justify-between gap-4">
            <p className="text-sm text-muted-foreground">
              {t('googleCloudDns.enabledHint')}
            </p>
            <Switch
              checked={isEnabled}
              onCheckedChange={setEnabled}
              disabled={pending}
              aria-label={t('googleCloudDns.enabledLabel')}
            />
          </div>

          {settings.has_key ? (
            <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
              <dt className="text-muted-foreground">
                {t('googleCloudDns.project')}
              </dt>
              <dd className="min-w-0 break-all font-mono">
                {settings.project_id}
              </dd>
              <dt className="text-muted-foreground">
                {t('googleCloudDns.clientEmail')}
              </dt>
              <dd className="min-w-0 break-all font-mono">
                {settings.client_email}
              </dd>
            </dl>
          ) : null}

          <Field>
            <FieldLabel htmlFor="google-cloud-dns-key">
              {t('googleCloudDns.keyLabel')}
            </FieldLabel>
            <Textarea
              id="google-cloud-dns-key"
              autoComplete="off"
              spellCheck={false}
              rows={4}
              className="font-mono"
              value={keyJson}
              onChange={(e) => {
                setKeyJson(e.target.value)
              }}
              disabled={pending}
              placeholder={
                settings.has_key
                  ? t('googleCloudDns.keyPlaceholderSaved')
                  : t('googleCloudDns.keyPlaceholder')
              }
            />
            <FieldDescription>
              {settings.has_key
                ? t('googleCloudDns.keySaved')
                : t('googleCloudDns.keyNone')}
            </FieldDescription>
          </Field>

          <SettingsFormAlerts
            alerts={[
              formError ? { key: 'form', message: formError } : null,
              update.isError
                ? { key: 'update', message: update.error.message }
                : null,
              test.isError
                ? { key: 'test', message: test.error.message }
                : null,
              test.data && !test.data.ok
                ? {
                    key: 'testfail',
                    message: t('googleCloudDns.testFailed', {
                      error: test.data.error,
                    }),
                  }
                : null,
            ]}
          />

          {test.data?.ok ? (
            <p className="text-sm text-foreground" role="status">
              {t('googleCloudDns.testOk', {
                count: test.data.zones?.length ?? 0,
                zones: (test.data.zones ?? []).join(', '),
              })}
            </p>
          ) : null}

          <div className="flex flex-wrap items-center gap-2">
            <Button type="submit" size="sm" disabled={pending}>
              {update.isPending
                ? t('googleCloudDns.saving')
                : t('googleCloudDns.save')}
            </Button>
            {settings.has_key ? (
              <>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={pending || test.isPending}
                  onClick={() => {
                    test.mutate()
                  }}
                >
                  {test.isPending
                    ? t('googleCloudDns.testing')
                    : t('googleCloudDns.test')}
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={pending}
                  onClick={handleDisconnect}
                >
                  {t('googleCloudDns.disconnect')}
                </Button>
              </>
            ) : null}
          </div>
        </form>
      </CardContent>
    </Card>
  )
}
