import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { SettingsGroupId } from './groups'

// scroll-mt keeps the heading clear of the sticky section nav on jump.
export function SettingsGroup({
  id,
  children,
}: {
  id: SettingsGroupId
  children: ReactNode
}) {
  const { t } = useTranslation('common')
  const headingId = `settings-group-${id}`
  return (
    <section id={id} aria-labelledby={headingId} className="scroll-mt-20">
      <div className="mb-3">
        <h2 id={headingId} className="text-base font-semibold text-foreground">
          {t(`deploySettings.groups.${id}.title`)}
        </h2>
        <p className="text-sm text-muted-foreground">
          {t(`deploySettings.groups.${id}.description`)}
        </p>
      </div>
      <div className="space-y-4">{children}</div>
    </section>
  )
}
