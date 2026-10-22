import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { cardAnchor, type SettingsCardKey } from './groups'

// Wraps an existing card with its one-line "What this does" so the card
// internals stay untouched. recommended adds the defaults hint for cards
// whose empty fields fall back to a server default.
export function CardHelp({
  card,
  recommended = false,
  children,
}: {
  card: SettingsCardKey
  recommended?: boolean
  children: ReactNode
}) {
  const { t } = useTranslation('common')
  return (
    <div id={cardAnchor(card)} className="scroll-mt-20 space-y-1.5">
      <p className="px-1 text-xs text-muted-foreground">
        <span className="font-medium text-foreground">
          {t('deploySettings.whatItDoes')}
        </span>{' '}
        {t(`deploySettings.cards.${card}`)}
        {recommended ? ` ${t('deploySettings.recommended')}` : null}
      </p>
      {children}
    </div>
  )
}
