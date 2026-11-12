import { useId, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { useChangeLanguage } from '../../hooks/useChangeLanguage'
import { LanguageSelect } from '../language/LanguageSelect'

/** LanguageStep previews the language as the choice changes; saving happens when Continue is pressed, and Enter accepts the preselected one. */
export function LanguageStep({ onAccept }: { onAccept: () => void }) {
  const { t } = useTranslation('onboarding')
  const { language, preview } = useChangeLanguage()
  const id = useId()

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key !== 'Enter' || e.defaultPrevented) return
    if (e.target instanceof HTMLButtonElement) return
    e.preventDefault()
    onAccept()
  }

  return (
    <div className="space-y-2" onKeyDown={onKeyDown}>
      <label htmlFor={id} className="text-sm font-medium text-foreground">
        {t('language.label')}
      </label>
      <LanguageSelect id={id} value={language} onChange={preview} />
      <p className="text-xs text-muted-foreground">{t('language.hint')}</p>
    </div>
  )
}
