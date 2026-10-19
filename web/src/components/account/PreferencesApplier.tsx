import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useRouter } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useTheme } from '../ThemeProvider'
import { preferencesQueryOptions } from '../../queries/accountSelf'
import { isSupportedLanguage } from '../../lib/languages'
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

  const timezone = data?.timezone
  const language = data?.language
  const theme = data?.theme
  const landing = data?.landing_page

  useEffect(() => {
    if (timezone !== undefined) setUserTimezone(timezone)
  }, [timezone])

  useEffect(() => {
    if (language && isSupportedLanguage(i18n, language)) {
      void i18n.changeLanguage(language)
    }
  }, [language, i18n])

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
