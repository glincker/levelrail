import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import {
  EnvelopeIcon,
  PaperPlaneTiltIcon,
  SparkleIcon,
  WebhooksLogoIcon,
} from '@phosphor-icons/react/dist/ssr'
import { BrandLogoBadge } from './BrandLogoBadge'
import type { EmailSettings } from '../queries/emailSettings'
import {
  useSendTestEmail,
  useUpdateEmailSettings,
} from '../queries/emailSettings'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button, buttonVariants } from '@/components/ui/button'
import { Banner } from '@/components/ui/banner'
import { Card, CardContent } from '@/components/ui/card'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'

// Brand mark per backend, lazy-loaded via the same templateLogos.ts
// registry the catalog's own TemplateLogo uses (one dynamic import per
// logo, kept out of the initial bundle). SMTP keeps the generic
// EnvelopeIcon: it's a protocol, not a company, so no logo fits it.
function BackendIcon({ logoId }: { logoId?: string }) {
  return (
    <BrandLogoBadge
      logoId={logoId}
      className="size-6 p-0.5"
      fallback={<EnvelopeIcon className="size-4 text-muted-foreground" />}
    />
  )
}

const BACKEND_LOGO_ID: Record<string, string | undefined> = {
  smtp: undefined,
  ses: 'aws-ses',
  resend: 'resend',
}

const BACKEND_VALUES = ['smtp', 'ses', 'resend'] as const

// Well-known SMTP providers, prefilled on click so a user doesn't have
// to look up host/port by hand. All take the same generic SMTP relay
// this app already speaks; no separate native integration needed.
const SMTP_PRESETS = [
  { name: 'Gmail', logoId: 'gmail', host: 'smtp.gmail.com', port: 587 },
  { name: 'Mailgun', logoId: 'mailgun', host: 'smtp.mailgun.org', port: 587 },
  {
    name: 'Postmark',
    logoId: 'postmark',
    host: 'smtp.postmarkapp.com',
    port: 587,
  },
  {
    name: 'Brevo',
    logoId: 'brevo',
    host: 'smtp-relay.brevo.com',
    port: 587,
  },
  {
    name: 'Mailtrap',
    logoId: 'mailtrap',
    host: 'live.smtp.mailtrap.io',
    port: 587,
  },
] as const

// Mirrors validateEmailSettingsRequest (internal/api/email_settings.go):
import {
  emailSettingsSchema,
  toEmailSettings,
  toFieldValues,
  type EmailSettingsFormInput,
  type EmailSettingsFormOutput,
} from './EmailSettingsSchema'

// Platform-wide email backend (SMTP, AWS SES, or Resend): GET/PUT
// /api/v1/settings/email. Used by both internal/alerting's
// notifications and the forgot-password flow.
export function EmailSettingsCard({ settings }: { settings: EmailSettings }) {
  const { t } = useTranslation('settings')
  const updateSettings = useUpdateEmailSettings()
  const sendTestEmail = useSendTestEmail()
  const [testTo, setTestTo] = useState('')
  const { control, register, handleSubmit, formState, setValue } = useForm<
    EmailSettingsFormInput,
    unknown,
    EmailSettingsFormOutput
  >({
    resolver: zodResolver(emailSettingsSchema),
    values: toFieldValues(settings),
    resetOptions: { keepDirtyValues: true },
  })

  // All options rendered up front, not hidden behind a dropdown: a
  // dropdown hides that SES/Resend are options at all until opened.
  const backendOptions = BACKEND_VALUES.map((value) => ({
    value,
    label: t(`email.${value}.label`),
  }))

  const onSubmit = handleSubmit((values) => {
    updateSettings.mutate(toEmailSettings(values), {
      onSuccess: () => {
        toast.add({ title: t('email.saveSuccess'), type: 'success' })
      },
    })
  })

  function onSendTest() {
    sendTestEmail.mutate(testTo, {
      onSuccess: () => {
        toast.add({
          title: t('email.test.success', { to: testTo }),
          type: 'success',
        })
      },
      onError: (error) => {
        toast.add({ title: error.message, type: 'error' })
      },
    })
  }

  return (
    <div className="space-y-6">
      <Card>
        <CardContent className="space-y-4">
          <form
            onSubmit={(e) => {
              void onSubmit(e)
            }}
            className="space-y-4"
          >
            <Field>
              <FieldLabel htmlFor="email-backend">
                {t('email.backendLabel')}
              </FieldLabel>
              <Controller
                control={control}
                name="backend"
                render={({ field }) => (
                  <div
                    id="email-backend"
                    role="radiogroup"
                    aria-label={t('email.backendLabel')}
                    className="grid grid-cols-1 gap-2 sm:grid-cols-3"
                  >
                    {backendOptions.map((opt) => {
                      const selected = field.value === opt.value
                      return (
                        <button
                          key={opt.value}
                          type="button"
                          role="radio"
                          aria-checked={selected}
                          onClick={() => field.onChange(opt.value)}
                          className={cn(
                            'flex items-center gap-2 rounded-md border p-3 text-left text-sm font-medium transition-colors',
                            selected
                              ? 'border-primary bg-primary/5 text-foreground'
                              : 'border-border text-muted-foreground hover:bg-muted/50',
                          )}
                        >
                          <BackendIcon logoId={BACKEND_LOGO_ID[opt.value]} />
                          {opt.label}
                        </button>
                      )
                    })}
                  </div>
                )}
              />
            </Field>

            <Controller
              control={control}
              name="backend"
              render={({ field }) =>
                field.value === 'smtp' ? (
                  <FieldGroup className="gap-4">
                    <div className="flex items-center gap-2 text-sm font-medium text-foreground">
                      <BackendIcon logoId={BACKEND_LOGO_ID.smtp} />
                      {t('email.smtp.label')}
                    </div>
                    <div>
                      <FieldDescription className="mb-1.5">
                        {t('email.smtp.presetsLabel')}
                      </FieldDescription>
                      <div className="flex flex-wrap gap-1.5">
                        {SMTP_PRESETS.map((preset) => (
                          <button
                            key={preset.name}
                            type="button"
                            onClick={() => {
                              setValue('smtpHost', preset.host, {
                                shouldDirty: true,
                              })
                              setValue('smtpPort', preset.port, {
                                shouldDirty: true,
                              })
                            }}
                            className="flex items-center gap-1.5 rounded-full border border-border px-2.5 py-1 text-xs text-muted-foreground transition-colors hover:border-primary hover:text-foreground"
                          >
                            <BackendIcon logoId={preset.logoId} />
                            {preset.name}
                          </button>
                        ))}
                      </div>
                    </div>
                    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                      <Field
                        data-invalid={
                          formState.errors.smtpHost ? true : undefined
                        }
                      >
                        <FieldLabel htmlFor="smtp-host">
                          {t('email.smtp.host')}
                        </FieldLabel>
                        <Input
                          id="smtp-host"
                          {...register('smtpHost')}
                          placeholder="smtp.example.com"
                          aria-invalid={!!formState.errors.smtpHost}
                        />
                        <FieldError errors={[formState.errors.smtpHost]} />
                      </Field>
                      <Field
                        data-invalid={
                          formState.errors.smtpPort ? true : undefined
                        }
                      >
                        <FieldLabel htmlFor="smtp-port">
                          {t('email.smtp.port')}
                        </FieldLabel>
                        <Input
                          id="smtp-port"
                          type="number"
                          {...register('smtpPort')}
                          placeholder="587"
                          aria-invalid={!!formState.errors.smtpPort}
                        />
                        <FieldError errors={[formState.errors.smtpPort]} />
                      </Field>
                      <Field
                        data-invalid={
                          formState.errors.smtpUsername ? true : undefined
                        }
                      >
                        <FieldLabel htmlFor="smtp-username">
                          {t('email.smtp.username')}
                        </FieldLabel>
                        <Input
                          id="smtp-username"
                          {...register('smtpUsername')}
                          aria-invalid={!!formState.errors.smtpUsername}
                        />
                      </Field>
                      <Field
                        data-invalid={
                          formState.errors.smtpFrom ? true : undefined
                        }
                      >
                        <FieldLabel htmlFor="smtp-from">
                          {t('email.smtp.from')}
                        </FieldLabel>
                        <Input
                          id="smtp-from"
                          {...register('smtpFrom')}
                          placeholder="no-reply@example.com"
                          aria-invalid={!!formState.errors.smtpFrom}
                        />
                        <FieldError errors={[formState.errors.smtpFrom]} />
                      </Field>
                    </div>
                    <Field
                      data-invalid={
                        formState.errors.smtpPassword ? true : undefined
                      }
                    >
                      <FieldLabel htmlFor="smtp-password">
                        {t('email.smtp.password')}
                      </FieldLabel>
                      <Input
                        id="smtp-password"
                        type="password"
                        {...register('smtpPassword')}
                        aria-invalid={!!formState.errors.smtpPassword}
                      />
                      <FieldDescription>
                        {settings.smtp_password_set
                          ? t('email.smtp.passwordSet')
                          : t('email.smtp.passwordUnset')}
                      </FieldDescription>
                    </Field>
                  </FieldGroup>
                ) : field.value === 'ses' ? (
                  <FieldGroup className="gap-4">
                    <div className="flex items-center gap-2 text-sm font-medium text-foreground">
                      <BackendIcon logoId={BACKEND_LOGO_ID.ses} />
                      {t('email.ses.label')}
                    </div>
                    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                      <Field
                        data-invalid={
                          formState.errors.sesRegion ? true : undefined
                        }
                      >
                        <FieldLabel htmlFor="ses-region">
                          {t('email.ses.region')}
                        </FieldLabel>
                        <Input
                          id="ses-region"
                          {...register('sesRegion')}
                          placeholder="us-east-1"
                          aria-invalid={!!formState.errors.sesRegion}
                        />
                        <FieldError errors={[formState.errors.sesRegion]} />
                      </Field>
                      <Field
                        data-invalid={
                          formState.errors.sesAccessKeyId ? true : undefined
                        }
                      >
                        <FieldLabel htmlFor="ses-access-key-id">
                          {t('email.ses.accessKeyId')}
                        </FieldLabel>
                        <Input
                          id="ses-access-key-id"
                          {...register('sesAccessKeyId')}
                          aria-invalid={!!formState.errors.sesAccessKeyId}
                        />
                        <FieldError
                          errors={[formState.errors.sesAccessKeyId]}
                        />
                      </Field>
                      <Field
                        data-invalid={
                          formState.errors.sesFrom ? true : undefined
                        }
                      >
                        <FieldLabel htmlFor="ses-from">
                          {t('email.ses.from')}
                        </FieldLabel>
                        <Input
                          id="ses-from"
                          {...register('sesFrom')}
                          placeholder="no-reply@example.com"
                          aria-invalid={!!formState.errors.sesFrom}
                        />
                        <FieldError errors={[formState.errors.sesFrom]} />
                      </Field>
                    </div>
                    <Field
                      data-invalid={
                        formState.errors.sesSecretAccessKey ? true : undefined
                      }
                    >
                      <FieldLabel htmlFor="ses-secret-access-key">
                        {t('email.ses.secretAccessKey')}
                      </FieldLabel>
                      <Input
                        id="ses-secret-access-key"
                        type="password"
                        {...register('sesSecretAccessKey')}
                        aria-invalid={!!formState.errors.sesSecretAccessKey}
                      />
                      <FieldDescription>
                        {settings.ses_secret_access_key_set
                          ? t('email.ses.secretSet')
                          : t('email.ses.secretUnset')}
                      </FieldDescription>
                    </Field>
                  </FieldGroup>
                ) : field.value === 'resend' ? (
                  <FieldGroup className="gap-4">
                    <div className="flex items-center gap-2 text-sm font-medium text-foreground">
                      <BackendIcon logoId={BACKEND_LOGO_ID.resend} />
                      {t('email.resend.label')}
                    </div>
                    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                      <Field
                        data-invalid={
                          formState.errors.resendFrom ? true : undefined
                        }
                      >
                        <FieldLabel htmlFor="resend-from">
                          {t('email.resend.from')}
                        </FieldLabel>
                        <Input
                          id="resend-from"
                          {...register('resendFrom')}
                          placeholder="no-reply@example.com"
                          aria-invalid={!!formState.errors.resendFrom}
                        />
                        <FieldError errors={[formState.errors.resendFrom]} />
                      </Field>
                      <Field
                        data-invalid={
                          formState.errors.resendApiKey ? true : undefined
                        }
                      >
                        <FieldLabel htmlFor="resend-api-key">
                          {t('email.resend.apiKey')}
                        </FieldLabel>
                        <Input
                          id="resend-api-key"
                          type="password"
                          {...register('resendApiKey')}
                          aria-invalid={!!formState.errors.resendApiKey}
                        />
                        <FieldDescription>
                          {settings.resend_api_key_set
                            ? t('email.resend.apiKeySet')
                            : t('email.resend.apiKeyUnset')}
                        </FieldDescription>
                      </Field>
                    </div>
                  </FieldGroup>
                ) : (
                  <></>
                )
              }
            />

            <div className="flex items-center gap-2">
              <Button
                type="submit"
                size="sm"
                disabled={updateSettings.isPending}
              >
                {updateSettings.isPending ? t('email.saving') : t('email.save')}
              </Button>
            </div>
            {updateSettings.isError ? (
              <Alert variant="destructive">
                <AlertDescription>
                  {updateSettings.error.message}
                </AlertDescription>
              </Alert>
            ) : null}
          </form>

          {settings.backend ? (
            <FieldGroup className="gap-2 border-t border-border pt-4">
              <FieldLabel htmlFor="test-email-to">
                {t('email.test.title')}
              </FieldLabel>
              <FieldDescription>{t('email.test.description')}</FieldDescription>
              <div className="flex flex-col gap-2 sm:flex-row">
                <Input
                  id="test-email-to"
                  type="email"
                  value={testTo}
                  onChange={(e) => setTestTo(e.target.value)}
                  placeholder={t('email.test.placeholder')}
                  className="sm:max-w-xs"
                />
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={!testTo || sendTestEmail.isPending}
                  onClick={onSendTest}
                >
                  <PaperPlaneTiltIcon className="size-3.5" />
                  {sendTestEmail.isPending
                    ? t('email.test.sending')
                    : t('email.test.send')}
                </Button>
              </div>
            </FieldGroup>
          ) : null}
        </CardContent>
      </Card>

      <p className="flex items-center gap-2 text-xs text-muted-foreground">
        <SparkleIcon className="size-4 shrink-0" aria-hidden="true" />
        <span>{t('email.comingSoon')}</span>
      </p>

      <Banner
        variant="info"
        icon={<WebhooksLogoIcon />}
        title={t('email.channelsCta')}
        action={
          <Link
            to="/settings/notification-channels"
            className={buttonVariants({ variant: 'outline', size: 'sm' })}
          >
            {t('email.channelsLink')}
          </Link>
        }
      />
    </div>
  )
}
