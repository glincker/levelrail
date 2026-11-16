import { useChangeLanguage } from '../../hooks/useChangeLanguage'
import { LanguageMenu } from './LanguageMenu'

/** HeaderLanguageSelect is the compact switcher for bars: it applies the language at once and saves it when possible. */
export function HeaderLanguageSelect({ className }: { className?: string }) {
  const { language, change, saving } = useChangeLanguage()
  return (
    <LanguageMenu
      value={language}
      disabled={saving}
      className={className}
      onChange={(code) => void change(code)}
    />
  )
}
