import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from '@/components/ui/toast'
import {
  accountSelfKeys,
  preferencesQueryOptions,
  type Preferences,
  useSavePreferences,
} from '../queries/accountSelf'
import {
  clearChosenLanguage,
  isSupportedLanguage,
  rememberChosenLanguage,
  resolveSupportedLanguage,
} from '../lib/languages'

export interface ChangeLanguage {
  /** Active language, mapped onto a supported code. */
  language: string
  /** Retranslates the UI now without saving anything (live preview). */
  preview: (code: string) => void
  /** Applies the language, then saves it to the account or, when signed out, to this browser. */
  change: (code: string) => Promise<void>
  /** Saves the language already shown; same persistence rules as change. */
  save: (code: string) => Promise<void>
  saving: boolean
}

/** useChangeLanguage is the one entry point for switching the dashboard language, shared by the header, login, onboarding, settings and the palette. */
export function useChangeLanguage(): ChangeLanguage {
  const { t, i18n } = useTranslation('common')
  const qc = useQueryClient()
  useQuery(preferencesQueryOptions())
  const savePrefs = useSavePreferences()

  const preview = (code: string) => {
    if (isSupportedLanguage(i18n, code)) void i18n.changeLanguage(code)
  }

  const save = async (code: string) => {
    if (!isSupportedLanguage(i18n, code)) return
    const current = qc.getQueryData<Preferences>(accountSelfKeys.preferences)
    if (!current) {
      rememberChosenLanguage(code)
      return
    }
    clearChosenLanguage()
    if (current.language === code) return
    try {
      await savePrefs.mutateAsync({ ...current, language: code })
    } catch {
      toast.add({ title: t('language.saveFailed'), type: 'error' })
    }
  }

  const change = async (code: string) => {
    if (!isSupportedLanguage(i18n, code)) return
    await i18n.changeLanguage(code)
    await save(code)
  }

  return {
    language: resolveSupportedLanguage(i18n),
    preview,
    change,
    save,
    saving: savePrefs.isPending,
  }
}
