import { useTranslation } from 'react-i18next'
import { ShieldCheckIcon } from '@phosphor-icons/react/dist/ssr'
import type { IngressSettings } from '../queries/domains'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'

export function IngressUpstreamFields({
  settings,
  upstream,
  onUpstreamChange,
  port,
  onPortChange,
  invalid,
}: {
  settings: IngressSettings
  upstream: boolean
  onUpstreamChange: (next: boolean) => void
  port: string
  onPortChange: (next: string) => void
  invalid?: boolean
}) {
  const { t } = useTranslation('domains')

  return (
    <div className="space-y-3 rounded-md border border-border p-3">
      {settings.tls_terminated_upstream ? (
        <Alert>
          <ShieldCheckIcon className="size-4" />
          <AlertTitle>{t('ingressUpstream.bannerTitle')}</AlertTitle>
          <AlertDescription>
            {t('ingressUpstream.bannerBody')}
            {settings.trusted_proxies_missing
              ? ` ${t('ingressUpstream.trustedProxiesMissing')}`
              : ''}
          </AlertDescription>
        </Alert>
      ) : null}
      <Field orientation="horizontal">
        <Switch
          id="tls-terminated-upstream"
          checked={upstream}
          onCheckedChange={onUpstreamChange}
        />
        <FieldLabel htmlFor="tls-terminated-upstream">
          {t('ingressUpstream.switchLabel')}
        </FieldLabel>
      </Field>
      <FieldDescription>
        {upstream && settings.acme_enabled
          ? t('ingressUpstream.acmeSkipped')
          : t('ingressUpstream.switchHelp')}
      </FieldDescription>
      <Field>
        <FieldLabel htmlFor="public-https-port">
          {t('ingressUpstream.portLabel')}
        </FieldLabel>
        <Input
          id="public-https-port"
          inputMode="numeric"
          className="w-32 font-mono"
          placeholder="443"
          value={port}
          onChange={(e) => {
            onPortChange(e.target.value)
          }}
        />
        <FieldDescription>{t('ingressUpstream.portHelp')}</FieldDescription>
        {invalid ? (
          <FieldError
            errors={[{ message: t('ingressUpstream.portInvalid') }]}
          />
        ) : null}
      </Field>
    </div>
  )
}
