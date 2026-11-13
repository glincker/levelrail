import { useChangeLanguage } from '../../hooks/useChangeLanguage'
import { LanguageSelect } from './LanguageSelect'

/** HeaderLanguageSelect is the compact switcher for bars: it applies the language at once and saves it when possible. */
export function HeaderLanguageSelect({ className }: { className?: string }) {
  const { language, change, saving } = useChangeLanguage()
  return (
    <LanguageSelect
      variant="compact"
      value={language}
      disabled={saving}
      className={className}
      onChange={(code) => void change(code)}
    />
  )
}
