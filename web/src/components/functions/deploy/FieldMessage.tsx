import { useTranslation } from 'react-i18next'
import type { ValidationKey } from './logic'

export function FieldError({
  id,
  messageKey,
}: Readonly<{ id: string; messageKey?: ValidationKey }>) {
  const { t } = useTranslation('functions')
  if (!messageKey) return null
  return (
    <p id={id} role="alert" className="text-xs text-destructive">
      {t(`deploy.${messageKey}`)}
    </p>
  )
}

export function FieldHelp({
  id,
  children,
}: Readonly<{ id?: string; children: React.ReactNode }>) {
  return (
    <p id={id} className="text-xs text-muted-foreground">
      {children}
    </p>
  )
}
