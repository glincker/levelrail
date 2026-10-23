import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  useCreateEmailProvider,
  useUpdateEmailProvider,
  type EmailProvider,
} from '../../queries/emailProviders'
import { EmailField } from './EmailField'
import { EmailSecretField } from './EmailSecretField'
import { ProviderMark } from './ProviderMark'
import {
  defaultProviderValues,
  emailProviderSchema,
  FROM_OVERRIDE_BACKENDS,
  SMTP_BACKENDS,
  toProviderPayload,
  type EmailForm,
  type EmailProviderFormInput,
  type EmailProviderFormOutput,
} from './emailProviderForm'
import { EMAIL_PROVIDERS, SES_SMTP_PORTS, SMTP_PRESETS } from './emailProviders'
import { INPUT_RECESSED, PRESS } from './emailStyles'

interface Props {
  /** The stored provider being edited; undefined while adding. */
  provider?: EmailProvider
  /** Which slot an added provider will take, for the heading. */
  role: 'primary' | 'backup'
  sharedFrom: string
  sesRegions: string[]
  onClose: () => void
}

function RegionSelect({
  form,
  id,
  regions,
}: {
  form: EmailForm
  id: string
  regions: string[]
}) {
  const { t } = useTranslation('settings')
  return (
    <Controller
      control={form.control}
      name="sesRegion"
      render={({ field }) => (
        <Select value={field.value} onValueChange={field.onChange}>
          <SelectTrigger id={id} className={`w-full ${INPUT_RECESSED}`}>
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

/** EmailProviderPanel edits one provider and shows only the fields its backend uses. */
export function EmailProviderPanel({
  provider,
  role,
  sharedFrom,
  sesRegions,
  onClose,
}: Readonly<Props>) {
  const { t } = useTranslation('settings')
  const uid = useId()
  const create = useCreateEmailProvider()
  const update = useUpdateEmailProvider()
  const saving = create.isPending || update.isPending
  const error = create.error ?? update.error

  const form = useForm<
    EmailProviderFormInput,
    unknown,
    EmailProviderFormOutput
  >({
    resolver: zodResolver(emailProviderSchema),
    defaultValues: defaultProviderValues(provider?.backend, provider),
  })
  const { register, control, formState, watch, setValue } = form
  const err = formState.errors
  const backend = watch('backend')
  const region = watch('sesRegion')
  const sameBackend = !provider || provider.backend === backend
  // Stored credential flags only describe the backend the row was saved with.
  const stored = sameBackend ? provider : undefined
  const providerId = sameBackend ? provider?.id : undefined
  const fid = (name: string) => `${uid}-${name}`
  const needsSharedFrom =
    !sharedFrom &&
    (!FROM_OVERRIDE_BACKENDS.has(backend) || !watch('fromOverride'))

  const onSubmit = form.handleSubmit((values) => {
    const payload = toProviderPayload(values, provider?.enabled)
    if (provider?.id) {
      update.mutate({ ...payload, id: provider.id }, { onSuccess: onClose })
    } else {
      create.mutate(payload, { onSuccess: onClose })
    }
  })

  const heading = provider
    ? t('emailProviders.panel.editTitle', {
        role: t(`emailProviders.roles.${provider.role ?? role}`),
      })
    : t('emailProviders.panel.addTitle', {
        role: t(`emailProviders.roles.${role}`),
      })

  return (
    <form
      onSubmit={(e) => {
        void onSubmit(e)
      }}
      aria-label={heading}
      className="space-y-3 rounded-lg border border-border bg-background/60 p-3 shadow-inner"
    >
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        <EmailField
          id={fid('backend')}
          label={t('emailProviders.panel.backend')}
        >
          <Controller
            control={control}
            name="backend"
            render={({ field }) => (
              <Select
                value={field.value}
                onValueChange={(v) => {
                  field.onChange(v)
                }}
              >
                <SelectTrigger
                  id={fid('backend')}
                  className={`w-full ${INPUT_RECESSED}`}
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {EMAIL_PROVIDERS.map((p) => (
                    <SelectItem key={p.id} value={p.id}>
                      <span className="flex items-center gap-2">
                        <ProviderMark
                          logoId={p.logoId}
                          label={t(`emailDelivery.providers.${p.labelKey}`)}
                          generic={p.id === 'smtp'}
                        />
                        {t(`emailDelivery.providers.${p.labelKey}`)}
                      </span>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          />
        </EmailField>
        <EmailField
          id={fid('name')}
          label={t('emailProviders.panel.name')}
          description={t('emailProviders.panel.nameHelp')}
        >
          <Input
            id={fid('name')}
            className={INPUT_RECESSED}
            {...register('name')}
          />
        </EmailField>
        {FROM_OVERRIDE_BACKENDS.has(backend) ? (
          <EmailField
            id={fid('from')}
            label={t('emailProviders.panel.fromOverride')}
            description={
              sharedFrom
                ? t('emailProviders.panel.fromOverrideHelp', {
                    from: sharedFrom,
                  })
                : t('emailProviders.panel.fromOverrideNoShared')
            }
          >
            <Input
              id={fid('from')}
              type="email"
              className={INPUT_RECESSED}
              {...register('fromOverride')}
            />
          </EmailField>
        ) : null}
      </div>

      {backend === 'smtp' ? (
        <div className="space-y-2">
          <div className="flex flex-wrap items-center gap-1.5">
            <span className="text-xs text-muted-foreground">
              {t('emailDelivery.smtp.presets')}
            </span>
            {SMTP_PRESETS.map((preset) => (
              <button
                key={preset.name}
                type="button"
                onClick={() => {
                  setValue('smtpHost', preset.host, { shouldDirty: true })
                  setValue('smtpPort', preset.port, { shouldDirty: true })
                }}
                className={`flex items-center gap-1.5 rounded-full border border-border px-2 py-0.5 text-xs text-muted-foreground transition-colors hover:border-primary hover:text-foreground ${PRESS}`}
              >
                <ProviderMark logoId={preset.logoId} label={preset.name} />
                {preset.name}
              </button>
            ))}
          </div>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <EmailField
              id={fid('smtp-host')}
              label={t('emailDelivery.smtp.host')}
              error={err.smtpHost}
            >
              <Input
                id={fid('smtp-host')}
                className={INPUT_RECESSED}
                placeholder={t('emailDelivery.smtp.hostPlaceholder')}
                {...register('smtpHost')}
              />
            </EmailField>
            <EmailField
              id={fid('smtp-port')}
              label={t('emailDelivery.smtp.port')}
              error={err.smtpPort}
            >
              <Input
                id={fid('smtp-port')}
                type="number"
                className={INPUT_RECESSED}
                {...register('smtpPort')}
              />
            </EmailField>
            <EmailField
              id={fid('smtp-user')}
              label={t('emailDelivery.username')}
            >
              <Input
                id={fid('smtp-user')}
                className={INPUT_RECESSED}
                {...register('smtpUsername')}
              />
            </EmailField>
            <EmailSecretField
              form={form}
              id={fid('smtp-password')}
              name="smtpPassword"
              kind="smtp_password"
              label={t('emailDelivery.password')}
              providerId={providerId}
              isSet={stored?.smtp_password_set}
            />
          </div>
        </div>
      ) : null}

      {backend === 'ses' ? (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <EmailField
            id={fid('ses-region')}
            label={t('emailDelivery.ses.region')}
            error={err.sesRegion}
            description={t('emailDelivery.ses.regionHelp')}
          >
            <RegionSelect
              form={form}
              id={fid('ses-region')}
              regions={sesRegions}
            />
          </EmailField>
          <EmailField
            id={fid('ses-key-id')}
            label={t('emailDelivery.ses.accessKeyId')}
            error={err.sesAccessKeyId}
          >
            <Input
              id={fid('ses-key-id')}
              className={INPUT_RECESSED}
              {...register('sesAccessKeyId')}
            />
          </EmailField>
          <EmailSecretField
            form={form}
            id={fid('ses-secret')}
            name="sesSecretAccessKey"
            kind="ses_secret_access_key"
            label={t('emailDelivery.ses.secretAccessKey')}
            providerId={providerId}
            isSet={stored?.ses_secret_access_key_set}
          />
        </div>
      ) : null}

      {backend === 'ses_smtp' ? (
        <div className="space-y-2">
          <p className="text-xs text-muted-foreground">
            {t('emailProviders.panel.sesSmtpNote')}
          </p>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <EmailField
              id={fid('ses-region')}
              label={t('emailDelivery.ses.region')}
              error={err.sesRegion}
              description={
                region
                  ? t('emailDelivery.sesSmtp.derivedHost', { region })
                  : t('emailDelivery.ses.regionHelp')
              }
            >
              <RegionSelect
                form={form}
                id={fid('ses-region')}
                regions={sesRegions}
              />
            </EmailField>
            <EmailField
              id={fid('ses-port')}
              label={t('emailDelivery.smtp.port')}
            >
              <Controller
                control={control}
                name="sesSmtpPort"
                render={({ field }) => (
                  <Select
                    value={String(field.value)}
                    onValueChange={(v) => {
                      field.onChange(Number(v))
                    }}
                  >
                    <SelectTrigger
                      id={fid('ses-port')}
                      className={`w-full ${INPUT_RECESSED}`}
                    >
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
              id={fid('ses-user')}
              label={t('emailDelivery.username')}
              error={err.sesSmtpUsername}
            >
              <Input
                id={fid('ses-user')}
                className={INPUT_RECESSED}
                {...register('sesSmtpUsername')}
              />
            </EmailField>
            <EmailSecretField
              form={form}
              id={fid('ses-smtp-password')}
              name="sesSmtpPassword"
              kind="ses_smtp_password"
              label={t('emailDelivery.password')}
              providerId={providerId}
              isSet={stored?.ses_smtp_password_set}
            />
          </div>
        </div>
      ) : null}

      {backend === 'resend' ? (
        <div className="grid gap-3 sm:grid-cols-2">
          <EmailSecretField
            form={form}
            id={fid('resend-key')}
            name="resendApiKey"
            kind="resend_api_key"
            label={t('emailDelivery.apiKey')}
            providerId={providerId}
            isSet={stored?.resend_api_key_set}
          />
        </div>
      ) : null}

      {backend === 'postmark' ? (
        <div className="grid gap-3 sm:grid-cols-2">
          <EmailSecretField
            form={form}
            id={fid('postmark-token')}
            name="postmarkToken"
            kind="postmark_server_token"
            label={t('emailDelivery.postmark.token')}
            providerId={providerId}
            isSet={stored?.postmark_server_token_set}
          />
        </div>
      ) : null}

      {backend === 'sendgrid' ? (
        <div className="grid gap-3 sm:grid-cols-2">
          <EmailSecretField
            form={form}
            id={fid('sendgrid-key')}
            name="sendgridApiKey"
            kind="sendgrid_api_key"
            label={t('emailDelivery.apiKey')}
            providerId={providerId}
            isSet={stored?.sendgrid_api_key_set}
          />
        </div>
      ) : null}

      {backend === 'mailgun' ? (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <EmailField
            id={fid('mg-domain')}
            label={t('emailDelivery.mailgun.domain')}
            error={err.mailgunDomain}
          >
            <Input
              id={fid('mg-domain')}
              className={INPUT_RECESSED}
              placeholder={t('emailDelivery.mailgun.domainPlaceholder')}
              {...register('mailgunDomain')}
            />
          </EmailField>
          <EmailField
            id={fid('mg-region')}
            label={t('emailDelivery.mailgun.region')}
          >
            <Controller
              control={control}
              name="mailgunRegion"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger
                    id={fid('mg-region')}
                    className={`w-full ${INPUT_RECESSED}`}
                  >
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
          <EmailSecretField
            form={form}
            id={fid('mg-key')}
            name="mailgunApiKey"
            kind="mailgun_api_key"
            label={t('emailDelivery.apiKey')}
            providerId={providerId}
            isSet={stored?.mailgun_api_key_set}
          />
        </div>
      ) : null}

      {SMTP_BACKENDS.has(backend) ? (
        <div className="grid items-end gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <EmailField
            id={fid('tls-mode')}
            label={t('emailDelivery.security.tlsMode')}
          >
            <Controller
              control={control}
              name="tlsMode"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger
                    id={fid('tls-mode')}
                    className={`w-full ${INPUT_RECESSED}`}
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {(['auto', 'starttls', 'tls', 'none'] as const).map((m) => (
                      <SelectItem key={m} value={m}>
                        {t(`emailDelivery.security.modes.${m}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          </EmailField>
          <Controller
            control={control}
            name="tlsVerify"
            render={({ field }) => (
              <label className="flex h-8 items-center gap-2 text-sm">
                <Switch
                  size="sm"
                  checked={field.value}
                  onCheckedChange={field.onChange}
                  aria-label={t('emailDelivery.security.verify')}
                />
                {t('emailDelivery.security.verify')}
              </label>
            )}
          />
          {watch('tlsVerify') ? null : (
            <Alert variant="destructive" className="sm:col-span-2">
              <AlertDescription>
                {t('emailDelivery.security.verifyOff')}
              </AlertDescription>
            </Alert>
          )}
        </div>
      ) : null}

      {needsSharedFrom ? (
        <Alert>
          <AlertDescription>
            {t('emailProviders.panel.needsSender')}{' '}
            <a href="#email-from-address" className="underline">
              {t('emailProviders.panel.needsSenderLink')}
            </a>
          </AlertDescription>
        </Alert>
      ) : null}

      <div className="flex flex-wrap items-center gap-2">
        <Button type="submit" size="sm" className={PRESS} disabled={saving}>
          {saving
            ? t('email.saving')
            : provider
              ? t('emailProviders.panel.save')
              : t('emailProviders.panel.add')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className={PRESS}
          onClick={onClose}
        >
          {t('emailProviders.panel.cancel')}
        </Button>
        {error ? (
          <Alert variant="destructive" className="min-w-0 flex-1" role="alert">
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        ) : null}
      </div>
    </form>
  )
}
