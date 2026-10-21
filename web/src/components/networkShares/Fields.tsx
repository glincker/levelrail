import { useTranslation } from 'react-i18next'
import type { ShareErrorKey } from './logic'

export function FieldError({
  id,
  messageKey,
}: Readonly<{ id: string; messageKey?: ShareErrorKey }>) {
  const { t } = useTranslation('settings')
  if (!messageKey) return null
  return (
    <p id={id} role="alert" className="text-xs text-destructive">
      {t(`networkSharesPage.dialog.errors.${messageKey}`)}
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
