import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'

/** EmailField is a labelled field whose error is a settings:emailDelivery.validation key. */
export function EmailField({
  id,
  label,
  error,
  description,
  className,
  children,
}: {
  id: string
  label: ReactNode
  error?: { message?: string }
  description?: ReactNode
  className?: string
  children: ReactNode
}) {
  const { t } = useTranslation('settings')
  return (
    <Field data-invalid={error ? true : undefined} className={className}>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      {children}
      {description ? <FieldDescription>{description}</FieldDescription> : null}
      {error?.message ? (
        <FieldError>
          {t(`emailDelivery.${error.message}` as 'emailDelivery.title')}
        </FieldError>
      ) : null}
    </Field>
  )
}
