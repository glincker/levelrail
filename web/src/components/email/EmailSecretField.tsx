import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CircleCheckGlassIcon, KeyGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { SecretInput } from '@/components/ui/secret-input'
import type { EmailSecretField as SecretKind } from '../../queries/emailSecretReveal'
import { EmailField } from './EmailField'
import { RevealEmailSecret } from './RevealEmailSecret'
import { INPUT_RECESSED, PRESS } from './emailStyles'
import type { EmailForm, EmailProviderFormInput } from './emailProviderForm'

type SecretName =
  | 'smtpPassword'
  | 'sesSecretAccessKey'
  | 'sesSmtpPassword'
  | 'resendApiKey'
  | 'postmarkToken'
  | 'sendgridApiKey'
  | 'mailgunApiKey'

interface Props {
  form: EmailForm
  id: string
  name: SecretName & keyof EmailProviderFormInput
  kind: SecretKind
  label: string
  /** The stored provider this credential belongs to; undefined while adding. */
  providerId?: string
  isSet?: boolean
  /** Where to find this credential, shown while it is not stored yet. */
  hint?: string
  className?: string
}

/** EmailSecretField shows a stored credential as a Set chip with Replace and Reveal, never its value. */
export function EmailSecretField({
  form,
  id,
  name,
  kind,
  label,
  providerId,
  isSet,
  hint,
  className,
}: Readonly<Props>) {
  const { t } = useTranslation('settings')
  const [replacing, setReplacing] = useState(false)
  const stored = Boolean(isSet && providerId)

  if (stored && !replacing) {
    return (
      <div className={className}>
        <div className="text-sm font-medium">{label}</div>
        <div className="mt-1.5 flex flex-wrap items-start gap-2">
          <Badge variant="success" className="h-7">
            <CircleCheckGlassIcon
              className="size-4.5 text-tone-success"
              aria-hidden="true"
            />
            {t('emailProviders.secret.set')}
          </Badge>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className={PRESS}
            onClick={() => {
              setReplacing(true)
            }}
            aria-label={t('emailProviders.secret.replaceLabel', { label })}
          >
            <KeyGlassIcon size="1.4em" aria-hidden="true" />
            {t('emailProviders.secret.replace')}
          </Button>
          {providerId ? (
            <div className="min-w-0 flex-1 [&>div]:mt-0">
              <RevealEmailSecret providerId={providerId} field={kind} />
            </div>
          ) : null}
        </div>
      </div>
    )
  }

  return (
    <EmailField
      id={id}
      label={label}
      className={className}
      description={
        stored
          ? t('emailProviders.secret.replaceHint')
          : (hint ?? t('emailDelivery.secretUnset'))
      }
      trailing={
        stored ? (
          <Button
            type="button"
            variant="ghost"
            size="xs"
            className={`self-start ${PRESS}`}
            onClick={() => {
              form.resetField(name)
              setReplacing(false)
            }}
          >
            {t('emailProviders.secret.keep')}
          </Button>
        ) : null
      }
    >
      <SecretInput
        id={id}
        autoComplete="new-password"
        className={INPUT_RECESSED}
        {...form.register(name)}
      />
    </EmailField>
  )
}
