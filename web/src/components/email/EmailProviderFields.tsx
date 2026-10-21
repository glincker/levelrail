import { useTranslation } from 'react-i18next'
import { Controller, type UseFormReturn } from 'react-hook-form'
import { Banner } from '@/components/ui/banner'
import { FieldDescription } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { EmailSettings } from '../../queries/emailSettings'
import type {
  EmailSettingsFormInput,
  EmailSettingsFormOutput,
} from '../EmailSettingsSchema'
import { EmailField } from './EmailField'
import { ProviderMark } from './ProviderMark'
import { SES_SMTP_PORTS, SMTP_PRESETS } from './emailProviders'

export type EmailForm = UseFormReturn<
  EmailSettingsFormInput,
  unknown,
  EmailSettingsFormOutput
>

interface Props {
  form: EmailForm
  settings: EmailSettings
}

function SecretHint({ set }: { set?: boolean }) {
  const { t } = useTranslation('settings')
  return (
    <>{set ? t('emailDelivery.secretSet') : t('emailDelivery.secretUnset')}</>
  )
}

function SmtpFields({ form, settings }: Props) {
  const { t } = useTranslation('settings')
  const { register, setValue, formState } = form
  const err = formState.errors
  return (
    <div className="space-y-3">
      <div>
        <FieldDescription className="mb-1.5">
          {t('emailDelivery.smtp.presets')}
        </FieldDescription>
        <div className="flex flex-wrap gap-1.5">
          {SMTP_PRESETS.map((preset) => (
            <button
              key={preset.name}
              type="button"
              onClick={() => {
                setValue('smtpHost', preset.host, { shouldDirty: true })
                setValue('smtpPort', preset.port, { shouldDirty: true })
              }}
              className="flex items-center gap-1.5 rounded-full border border-border px-2.5 py-1 text-xs text-muted-foreground transition-colors hover:border-primary hover:text-foreground"
            >
              <ProviderMark logoId={preset.logoId} label={preset.name} />
              {preset.name}
            </button>
          ))}
        </div>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <EmailField
          id="smtp-host"
          label={t('emailDelivery.smtp.host')}
          error={err.smtpHost}
        >
          <Input
            id="smtp-host"
            {...register('smtpHost')}
            placeholder={t('emailDelivery.smtp.hostPlaceholder')}
          />
        </EmailField>
        <EmailField
          id="smtp-port"
          label={t('emailDelivery.smtp.port')}
          error={err.smtpPort}
        >
          <Input
            id="smtp-port"
            type="number"
            {...register('smtpPort')}
            placeholder="587"
          />
        </EmailField>
        <EmailField id="smtp-username" label={t('emailDelivery.username')}>
          <Input id="smtp-username" {...register('smtpUsername')} />
        </EmailField>
        <EmailField
          id="smtp-password"
          label={t('emailDelivery.password')}
          description={<SecretHint set={settings.smtp_password_set} />}
        >
          <Input
            id="smtp-password"
            type="password"
            autoComplete="new-password"
            {...register('smtpPassword')}
          />
        </EmailField>
      </div>
    </div>
  )
}

function SesApiFields({
  form,
  settings,
  regions,
}: Props & { regions: string[] }) {
  const { t } = useTranslation('settings')
  const { register, control, formState } = form
  const err = formState.errors
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <EmailField
        id="ses-region"
        label={t('emailDelivery.ses.region')}
        error={err.sesRegion}
        description={t('emailDelivery.ses.regionHelp')}
      >
        <RegionSelect control={control} regions={regions} />
      </EmailField>
      <EmailField
        id="ses-access-key-id"
        label={t('emailDelivery.ses.accessKeyId')}
        error={err.sesAccessKeyId}
      >
        <Input id="ses-access-key-id" {...register('sesAccessKeyId')} />
      </EmailField>
      <EmailField
        id="ses-secret-access-key"
        label={t('emailDelivery.ses.secretAccessKey')}
        description={<SecretHint set={settings.ses_secret_access_key_set} />}
        className="sm:col-span-2"
      >
        <Input
          id="ses-secret-access-key"
          type="password"
          autoComplete="new-password"
          {...register('sesSecretAccessKey')}
        />
      </EmailField>
    </div>
  )
}

function RegionSelect({
  control,
  regions,
}: {
  control: EmailForm['control']
  regions: string[]
}) {
  const { t } = useTranslation('settings')
  return (
    <Controller
      control={control}
      name="sesRegion"
      render={({ field }) => (
        <Select value={field.value} onValueChange={field.onChange}>
          <SelectTrigger id="ses-region" className="w-full">
            <SelectValue
              placeholder={t('emailDelivery.ses.regionPlaceholder')}
            />
          </SelectTrigger>
          <SelectContent>
            {regions.map((r) => (
              <SelectItem key={r} value={r}>
                {r}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
    />
  )
}

function SesSmtpFields({
  form,
  settings,
  regions,
}: Props & { regions: string[] }) {
  const { t } = useTranslation('settings')
  const { register, control, formState, watch } = form
  const err = formState.errors
  const region = watch('sesRegion')
  return (
    <div className="space-y-3">
      <Banner
        variant="info"
        title={t('emailDelivery.sesSmtp.credentialsTitle')}
      >
        {t('emailDelivery.sesSmtp.credentialsBody')}
      </Banner>
      <div className="grid gap-3 sm:grid-cols-2">
        <EmailField
          id="ses-region"
          label={t('emailDelivery.ses.region')}
          error={err.sesRegion}
          description={
            region
              ? t('emailDelivery.sesSmtp.derivedHost', { region })
              : t('emailDelivery.ses.regionHelp')
          }
        >
          <RegionSelect control={control} regions={regions} />
        </EmailField>
        <EmailField id="ses-smtp-port" label={t('emailDelivery.smtp.port')}>
          <Controller
            control={control}
            name="sesSmtpPort"
            render={({ field }) => (
              <Select
                value={String(field.value)}
                onValueChange={(v) => field.onChange(Number(v))}
              >
                <SelectTrigger id="ses-smtp-port" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {SES_SMTP_PORTS.map((p) => (
                    <SelectItem key={p} value={String(p)}>
                      {p === 587
                        ? t('emailDelivery.sesSmtp.port587')
                        : t('emailDelivery.sesSmtp.port465')}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          />
        </EmailField>
        <EmailField
          id="ses-smtp-username"
          label={t('emailDelivery.username')}
          error={err.sesSmtpUsername}
        >
          <Input id="ses-smtp-username" {...register('sesSmtpUsername')} />
        </EmailField>
        <EmailField
          id="ses-smtp-password"
          label={t('emailDelivery.password')}
          description={<SecretHint set={settings.ses_smtp_password_set} />}
        >
          <Input
            id="ses-smtp-password"
            type="password"
            autoComplete="new-password"
            {...register('sesSmtpPassword')}
          />
        </EmailField>
      </div>
    </div>
  )
}

function ApiKeyField({
  form,
  id,
  name,
  label,
  isSet,
}: {
  form: EmailForm
  id: string
  name: 'resendApiKey' | 'postmarkToken' | 'sendgridApiKey' | 'mailgunApiKey'
  label: string
  isSet?: boolean
}) {
  return (
    <EmailField id={id} label={label} description={<SecretHint set={isSet} />}>
      <Input
        id={id}
        type="password"
        autoComplete="new-password"
        {...form.register(name)}
      />
    </EmailField>
  )
}

function MailgunFields({ form, settings }: Props) {
  const { t } = useTranslation('settings')
  const { register, control, formState } = form
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <EmailField
        id="mailgun-domain"
        label={t('emailDelivery.mailgun.domain')}
        error={formState.errors.mailgunDomain}
      >
        <Input
          id="mailgun-domain"
          {...register('mailgunDomain')}
          placeholder={t('emailDelivery.mailgun.domainPlaceholder')}
        />
      </EmailField>
      <EmailField id="mailgun-region" label={t('emailDelivery.mailgun.region')}>
        <Controller
          control={control}
          name="mailgunRegion"
          render={({ field }) => (
            <Select value={field.value} onValueChange={field.onChange}>
              <SelectTrigger id="mailgun-region" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="us">
                  {t('emailDelivery.mailgun.us')}
                </SelectItem>
                <SelectItem value="eu">
                  {t('emailDelivery.mailgun.eu')}
                </SelectItem>
              </SelectContent>
            </Select>
          )}
        />
      </EmailField>
      <div className="sm:col-span-2">
        <ApiKeyField
          form={form}
          id="mailgun-api-key"
          name="mailgunApiKey"
          label={t('emailDelivery.apiKey')}
          isSet={settings.mailgun_api_key_set}
        />
      </div>
    </div>
  )
}

/** EmailProviderFields renders only the fields the chosen provider needs. */
export function EmailProviderFields({ form, settings }: Props) {
  const { t } = useTranslation('settings')
  const regions = settings.ses_regions ?? []
  switch (form.watch('backend')) {
    case 'smtp':
      return <SmtpFields form={form} settings={settings} />
    case 'ses':
      return <SesApiFields form={form} settings={settings} regions={regions} />
    case 'ses_smtp':
      return <SesSmtpFields form={form} settings={settings} regions={regions} />
    case 'resend':
      return (
        <ApiKeyField
          form={form}
          id="resend-api-key"
          name="resendApiKey"
          label={t('emailDelivery.apiKey')}
          isSet={settings.resend_api_key_set}
        />
      )
    case 'postmark':
      return (
        <ApiKeyField
          form={form}
          id="postmark-token"
          name="postmarkToken"
          label={t('emailDelivery.postmark.token')}
          isSet={settings.postmark_server_token_set}
        />
      )
    case 'sendgrid':
      return (
        <ApiKeyField
          form={form}
          id="sendgrid-api-key"
          name="sendgridApiKey"
          label={t('emailDelivery.apiKey')}
          isSet={settings.sendgrid_api_key_set}
        />
      )
    case 'mailgun':
      return <MailgunFields form={form} settings={settings} />
    default:
      return null
  }
}
