import { useTranslation } from 'react-i18next'
import { z } from 'zod'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { useAppForm } from '../../hooks/useAppForm'
import { useUnsavedGuard } from '../../hooks/useUnsavedGuard'
import { useEmailProviders } from '../../queries/emailProviders'
import {
  useUpdateEmailSettings,
  type EmailSettings,
} from '../../queries/emailSettings'
import { EmailField } from './EmailField'
import { SMTP_BACKENDS } from './emailProviderForm'
import { CARD_FLOAT, INPUT_RECESSED, PRESS } from './emailStyles'
import { SuggestionChip } from './SuggestionChip'
import { EMAIL_RE, useEmailSuggestions } from './useEmailSuggestions'

const optionalEmail = z
  .string()
  .trim()
  .refine((v) => v === '' || EMAIL_RE.test(v), 'validation.emailInvalid')

const schema = z.object({
  fromName: z.string().trim(),
  fromAddress: optionalEmail,
  replyTo: optionalEmail,
  envelopeFrom: optionalEmail,
  sendsPerMinute: z.coerce.number().int().min(0),
  sendsPerDay: z.coerce.number().int().min(0),
})
type FormInput = z.input<typeof schema>
type FormOutput = z.output<typeof schema>

function toValues(s: EmailSettings): FormInput {
  return {
    fromName: s.from_name ?? '',
    fromAddress: s.from_address ?? '',
    replyTo: s.reply_to ?? '',
    envelopeFrom: s.envelope_from ?? '',
    sendsPerMinute: s.sends_per_minute ?? 0,
    sendsPerDay: s.sends_per_day ?? 0,
  }
}

/** EmailSenderCard holds the identity every provider sends as, and the send caps. */
export function EmailSenderCard({
  settings,
}: Readonly<{ settings: EmailSettings }>) {
  const { t } = useTranslation('settings')
  const save = useUpdateEmailSettings()
  const { data: providers } = useEmailProviders()
  const suggestions = useEmailSuggestions()
  const smtpBased = (providers ?? []).some((p) => SMTP_BACKENDS.has(p.backend))
  const form = useAppForm<FormInput, FormOutput>(schema, {
    mode: 'onChange',
    values: toValues(settings),
    resetOptions: { keepDirtyValues: true },
  })
  const { register, formState, setError, setValue, watch } = form
  const unsavedGuard = useUnsavedGuard(formState.isDirty)
  const fromRequired = settings.backend !== ''
  const fromName = watch('fromName') ?? ''
  const fromAddress = watch('fromAddress') ?? ''
  const replyTo = watch('replyTo') ?? ''
  const envelopeFrom = watch('envelopeFrom') ?? ''
  const fill = (name: keyof FormInput, value: string) => {
    setValue(name, value, { shouldDirty: true, shouldValidate: true })
  }

  const domain = EMAIL_RE.test(fromAddress.trim())
    ? (fromAddress.trim().split('@')[1]?.toLowerCase() ?? null)
    : null
  const mailgun = (providers ?? []).find(
    (p) => p.backend === 'mailgun' && p.mailgun_domain,
  )
  const mailgunMismatch =
    domain && mailgun?.mailgun_domain
      ? !(
          domain === mailgun.mailgun_domain.toLowerCase() ||
          domain.endsWith(`.${mailgun.mailgun_domain.toLowerCase()}`)
        )
      : false

  const onSubmit = form.handleSubmit((v) => {
    if (fromRequired && !v.fromAddress) {
      setError('fromAddress', { message: 'validation.fromRequired' })
      return
    }
    save.mutate(
      {
        ...settings,
        from_name: v.fromName,
        from_address: v.fromAddress,
        reply_to: v.replyTo,
        envelope_from: v.envelopeFrom,
        sends_per_minute: v.sendsPerMinute,
        sends_per_day: v.sendsPerDay,
      },
      {
        onSuccess: () => {
          // Re-baseline so the next edit is dirty again.
          form.reset(v)
          toast.add({ title: t('email.saveSuccess'), type: 'success' })
        },
      },
    )
  })

  return (
    <Card size="sm" className={CARD_FLOAT}>
      {unsavedGuard}
      <CardContent>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          aria-label={t('emailSender.title')}
          className="space-y-2"
        >
          <h2 className="text-sm font-semibold">{t('emailSender.title')}</h2>
          <div className="grid gap-x-3 gap-y-2 sm:grid-cols-2 xl:grid-cols-3">
            <EmailField
              id="from-name"
              label={t('emailDelivery.sender.fromName')}
              trailing={
                <SuggestionChip
                  value={suggestions.fromName}
                  current={fromName}
                  onUse={(v) => {
                    fill('fromName', v)
                  }}
                />
              }
            >
              <Input
                id="from-name"
                className={INPUT_RECESSED}
                {...register('fromName')}
              />
            </EmailField>
            <EmailField
              id="email-from-address"
              label={t('emailDelivery.sender.fromAddress')}
              error={formState.errors.fromAddress}
              trailing={
                <SuggestionChip
                  value={suggestions.fromAddress}
                  current={fromAddress}
                  onUse={(v) => {
                    fill('fromAddress', v)
                  }}
                />
              }
            >
              <Input
                id="email-from-address"
                type="email"
                className={INPUT_RECESSED}
                placeholder={t('emailSender.fromPlaceholder')}
                {...register('fromAddress')}
              />
            </EmailField>
            <EmailField
              id="reply-to"
              label={t('emailDelivery.sender.replyTo')}
              error={formState.errors.replyTo}
              trailing={
                <SuggestionChip
                  value={suggestions.replyTo}
                  current={replyTo}
                  onUse={(v) => {
                    fill('replyTo', v)
                  }}
                />
              }
            >
              <Input
                id="reply-to"
                type="email"
                className={INPUT_RECESSED}
                placeholder={t('emailSender.optional')}
                {...register('replyTo')}
              />
            </EmailField>
            {smtpBased ? (
              <EmailField
                id="envelope-from"
                label={t('emailDelivery.sender.envelopeFrom')}
                error={formState.errors.envelopeFrom}
                trailing={
                  <SuggestionChip
                    value={
                      EMAIL_RE.test(fromAddress.trim())
                        ? fromAddress.trim()
                        : null
                    }
                    current={envelopeFrom}
                    onUse={(v) => {
                      fill('envelopeFrom', v)
                    }}
                  />
                }
              >
                <Input
                  id="envelope-from"
                  type="email"
                  className={INPUT_RECESSED}
                  placeholder={t('emailSender.envelopePlaceholder')}
                  {...register('envelopeFrom')}
                />
              </EmailField>
            ) : null}
            <EmailField
              id="sends-per-minute"
              label={t('emailDelivery.limits.perMinute')}
            >
              <Input
                id="sends-per-minute"
                type="number"
                min={0}
                className={INPUT_RECESSED}
                {...register('sendsPerMinute')}
              />
            </EmailField>
            <EmailField
              id="sends-per-day"
              label={t('emailDelivery.limits.perDay')}
            >
              <Input
                id="sends-per-day"
                type="number"
                min={0}
                className={INPUT_RECESSED}
                {...register('sendsPerDay')}
              />
            </EmailField>
            <div className="flex items-end">
              <Button
                type="submit"
                size="sm"
                className={PRESS}
                disabled={save.isPending}
              >
                {save.isPending ? t('email.saving') : t('email.save')}
              </Button>
            </div>
          </div>
          <p className="text-xs text-muted-foreground">
            {t('emailSender.capsHelp')}
          </p>
          {domain && (providers ?? []).length > 0 ? (
            <p
              className={
                mailgunMismatch
                  ? 'text-xs text-tone-warning'
                  : 'text-xs text-muted-foreground'
              }
            >
              {mailgunMismatch
                ? t('emailSender.mailgunMismatch', {
                    domain,
                    mailgun: mailgun?.mailgun_domain,
                  })
                : t('emailSender.domainHint', { domain })}{' '}
              <a href="#email-delivery-check" className="underline">
                {t('emailSender.domainHintLink')}
              </a>
            </p>
          ) : null}
          {save.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{save.error.message}</AlertDescription>
            </Alert>
          ) : null}
        </form>
      </CardContent>
    </Card>
  )
}
