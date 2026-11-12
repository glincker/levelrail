import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useRouter } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useTheme } from '../ThemeProvider'
import {
  preferencesQueryOptions,
  useSavePreferences,
} from '../../queries/accountSelf'
import {
  clearChosenLanguage,
  isSupportedLanguage,
  takeChosenLanguage,
} from '../../lib/languages'
import { setUserTimezone } from '../../lib/userPreferences'

const LANDING_DONE_KEY = 'levelrail-landing-applied'

function landingAlreadyApplied(): boolean {
  try {
    return window.sessionStorage.getItem(LANDING_DONE_KEY) === '1'
  } catch {
    return true
  }
}

function markLandingApplied(): void {
  try {
    window.sessionStorage.setItem(LANDING_DONE_KEY, '1')
  } catch {
    // Without storage the redirect simply never repeats.
  }
}

// Renders nothing: copies the signed-in user's saved preferences onto the
// running app (timezone, language, theme) and opens their landing page once
// per browser tab session.
export function PreferencesApplier() {
  const { data } = useQuery(preferencesQueryOptions())
  const { setTheme } = useTheme()
  const router = useRouter()
  const { i18n } = useTranslation()
  const { mutate: savePreferences } = useSavePreferences()

  const timezone = data?.timezone
  const language = data?.language
  const theme = data?.theme
  const landing = data?.landing_page

  useEffect(() => {
    if (timezone !== undefined) setUserTimezone(timezone)
  }, [timezone])

  // A language picked before sign-in is saved to the account once; otherwise the account value wins.
  useEffect(() => {
    if (!data) return
    const chosen = takeChosenLanguage()
    if (
      chosen !== '' &&
      chosen !== language &&
      isSupportedLanguage(i18n, chosen)
    ) {
      clearChosenLanguage()
      void i18n.changeLanguage(chosen)
      savePreferences({ language: chosen })
      return
    }
    clearChosenLanguage()
    if (language && isSupportedLanguage(i18n, language)) {
      void i18n.changeLanguage(language)
    }
  }, [data, language, i18n, savePreferences])

  useEffect(() => {
    if (theme) setTheme(theme)
  }, [theme, setTheme])

  useEffect(() => {
    if (landing === undefined || landingAlreadyApplied()) return
    markLandingApplied()
    if (landing !== '' && router.state.location.pathname === '/') {
      router.history.replace(landing)
    }
  }, [landing, router])

  return null
}
