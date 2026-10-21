import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { WebhooksLogoIcon } from '@phosphor-icons/react/dist/ssr'
import type { EmailSettings } from '../queries/emailSettings'
import { useUpdateEmailSettings } from '../queries/emailSettings'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Banner } from '@/components/ui/banner'
import { Button, buttonVariants } from '@/components/ui/button'
import { FormSection } from '@/components/ui/form-section'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import {
  emailSettingsSchema,
  toEmailSettings,
  toFieldValues,
  type EmailSettingsFormInput,
  type EmailSettingsFormOutput,
} from './EmailSettingsSchema'
import { EmailField } from './email/EmailField'
import { EmailProviderFields } from './email/EmailProviderFields'
import { ProviderMark } from './email/ProviderMark'
import { EMAIL_PROVIDERS } from './email/emailProviders'

const SMTP_BACKENDS = new Set(['smtp', 'ses_smtp'])

// Platform-wide delivery settings: GET/PUT /api/v1/settings/email, used by
// alert notifications, password resets and invites. Grouped as Sender,
// Provider, Delivery limits and Security.
export function EmailSettingsCard({ settings }: { settings: EmailSettings }) {
  const { t } = useTranslation('settings')
  const updateSettings = useUpdateEmailSettings()
  const form = useForm<
    EmailSettingsFormInput,
    unknown,
    EmailSettingsFormOutput
  >({
    resolver: zodResolver(emailSettingsSchema),
    values: toFieldValues(settings),
    resetOptions: { keepDirtyValues: true },
  })
  const { control, register, handleSubmit, formState, watch } = form
  const backend = watch('backend')
  const smtpBased = SMTP_BACKENDS.has(backend)

  const onSubmit = handleSubmit((values) => {
    updateSettings.mutate(toEmailSettings(values), {
      onSuccess: () => {
        toast.add({ title: t('email.saveSuccess'), type: 'success' })
      },
    })
  })

  return (
    <form
      onSubmit={(e) => {
        void onSubmit(e)
      }}
      className="space-y-2"
    >
      <FormSection
        title={t('emailDelivery.sender.title')}
        description={t('emailDelivery.sender.description')}
      >
        <div className="grid gap-3 sm:grid-cols-2">
          <EmailField id="from-name" label={t('emailDelivery.sender.fromName')}>
            <Input
              id="from-name"
              {...register('fromName')}
              placeholder="Acme Alerts"
            />
          </EmailField>
          <EmailField
            id="from-address"
            label={t('emailDelivery.sender.fromAddress')}
            error={formState.errors.fromAddress}
          >
            <Input
              id="from-address"
              type="email"
              {...register('fromAddress')}
              placeholder="no-reply@example.com"
            />
          </EmailField>
          <EmailField
            id="reply-to"
            label={t('emailDelivery.sender.replyTo')}
            description={t('emailDelivery.sender.optional')}
          >
            <Input id="reply-to" type="email" {...register('replyTo')} />
          </EmailField>
          {smtpBased ? (
            <EmailField
              id="envelope-from"
              label={t('emailDelivery.sender.envelopeFrom')}
              description={t('emailDelivery.sender.envelopeHelp')}
            >
              <Input
                id="envelope-from"
                type="email"
                {...register('envelopeFrom')}
              />
            </EmailField>
          ) : null}
        </div>
      </FormSection>

      <FormSection
        title={t('emailDelivery.provider.title')}
        description={t('emailDelivery.provider.description')}
      >
        <Controller
          control={control}
          name="backend"
          render={({ field }) => (
            <div
              role="radiogroup"
              aria-label={t('emailDelivery.provider.title')}
              className="grid grid-cols-1 gap-2 sm:grid-cols-2 xl:grid-cols-3"
            >
              {EMAIL_PROVIDERS.map((p) => {
                const label = t(`emailDelivery.providers.${p.labelKey}`)
                const selected = field.value === p.id
                return (
                  <button
                    key={p.id}
                    type="button"
                    role="radio"
                    aria-checked={selected}
                    onClick={() => field.onChange(p.id)}
                    className={cn(
                      'flex items-center gap-2 rounded-md border p-3 text-left text-sm font-medium transition-colors',
                      selected
                        ? 'border-primary bg-primary/5 text-foreground'
                        : 'border-border text-muted-foreground hover:bg-muted/50',
                    )}
                  >
                    <ProviderMark
                      logoId={p.logoId}
                      label={label}
                      generic={p.id === 'smtp'}
                    />
                    {label}
                  </button>
                )
              })}
            </div>
          )}
        />
        <EmailProviderFields form={form} settings={settings} />
      </FormSection>

      <FormSection
        title={t('emailDelivery.limits.title')}
        description={t('emailDelivery.limits.description')}
      >
        <div className="grid gap-3 sm:grid-cols-2">
          <EmailField
            id="sends-per-minute"
            label={t('emailDelivery.limits.perMinute')}
            description={t('emailDelivery.limits.zeroHelp')}
          >
            <Input
              id="sends-per-minute"
              type="number"
              min={0}
              {...register('sendsPerMinute')}
            />
          </EmailField>
          <EmailField
            id="sends-per-day"
            label={t('emailDelivery.limits.perDay')}
            description={t('emailDelivery.limits.zeroHelp')}
          >
            <Input
              id="sends-per-day"
              type="number"
              min={0}
              {...register('sendsPerDay')}
            />
          </EmailField>
        </div>
      </FormSection>

      <FormSection
        title={t('emailDelivery.security.title')}
        description={t('emailDelivery.security.description')}
      >
        {smtpBased ? (
          <div className="space-y-3">
            <EmailField
              id="tls-mode"
              label={t('emailDelivery.security.tlsMode')}
            >
              <Controller
                control={control}
                name="tlsMode"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="tls-mode" className="w-full sm:max-w-xs">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {(['auto', 'starttls', 'tls', 'none'] as const).map(
                        (m) => (
                          <SelectItem key={m} value={m}>
                            {t(`emailDelivery.security.modes.${m}`)}
                          </SelectItem>
                        ),
                      )}
                    </SelectContent>
                  </Select>
                )}
              />
            </EmailField>
            <Controller
              control={control}
              name="tlsVerify"
              render={({ field }) => (
                <label className="flex items-center gap-2 text-sm">
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    aria-label={t('emailDelivery.security.verify')}
                  />
                  {t('emailDelivery.security.verify')}
                </label>
              )}
            />
            {watch('tlsVerify') ? null : (
              <Alert variant="destructive">
                <AlertDescription>
                  {t('emailDelivery.security.verifyOff')}
                </AlertDescription>
              </Alert>
            )}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">
            {t('emailDelivery.security.apiNote')}
          </p>
        )}
      </FormSection>

      <div className="flex items-center gap-2 border-t border-border pt-4">
        <Button type="submit" size="sm" disabled={updateSettings.isPending}>
          {updateSettings.isPending ? t('email.saving') : t('email.save')}
        </Button>
        {updateSettings.isError ? (
          <Alert variant="destructive" className="flex-1">
            <AlertDescription>{updateSettings.error.message}</AlertDescription>
          </Alert>
        ) : null}
      </div>

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
    </form>
  )
}
