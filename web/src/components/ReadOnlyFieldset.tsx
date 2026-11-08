import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { useAbilities } from '../hooks/useAbilities'
import type { Ability } from '../types/token'

// UX-only gate: the server enforces the same ability on the matching
// endpoints. Disabled while the role is unknown so controls never flash on.
export function ReadOnlyFieldset({
  ability,
  children,
}: {
  ability: Ability
  children: ReactNode
}) {
  const { t } = useTranslation('common')
  const { role, can } = useAbilities()
  const allowed = can(ability)
  return (
    <>
      {role !== null && !allowed ? (
        <p className="text-muted-foreground text-xs">{t('readOnlyNotice')}</p>
      ) : null}
      <fieldset
        disabled={!allowed}
        className="m-0 min-w-0 space-y-4 border-0 p-0"
      >
        {children}
      </fieldset>
    </>
  )
}
