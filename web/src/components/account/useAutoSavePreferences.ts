import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from '@/components/ui/toast'
import { useTheme } from '../ThemeProvider'
import { useSavePreferences, type Preferences } from '../../queries/accountSelf'

/** useAutoSavePreferences saves every change at once and rolls the draft back if the server refuses it. */
export function useAutoSavePreferences(saved: Preferences) {
  const { t } = useTranslation('account')
  const save = useSavePreferences()
  const { setTheme } = useTheme()
  const [draft, setDraft] = useState<Preferences>(saved)

  // Another control (header, login) may change the language; follow it.
  const [seenLanguage, setSeenLanguage] = useState(saved.language)
  if (seenLanguage !== saved.language) {
    setSeenLanguage(saved.language)
    setDraft((d) => ({ ...d, language: saved.language }))
  }

  // The theme is applied before the round trip so the page does not lag the click.
  const apply = (next: Preferences) => {
    setDraft(next)
    if (next.theme !== draft.theme && next.theme !== '') setTheme(next.theme)
    const changes = Object.fromEntries(
      Object.entries(next).filter(
        ([k, v]) => v !== draft[k as keyof Preferences],
      ),
    ) as Partial<Preferences>
    save.mutate(changes, {
      onError: (e) => {
        setDraft(saved)
        toast.add({
          title: t('preferences.saveFailed'),
          description: e.message,
          type: 'error',
        })
      },
    })
  }
  const patch = (next: Partial<Preferences>) => apply({ ...draft, ...next })
  return { draft, apply, patch, save }
}
