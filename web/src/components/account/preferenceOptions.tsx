import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import {
  BellGlassIcon,
  DatabaseGlassIcon,
  GaugeGlassIcon,
  LayersGlassIcon,
  RocketGlassIcon,
  UserGlassIcon,
} from '@thesvg/ui-react'
import type { SearchSelectOption } from '@/components/ui/search-select'
import { browserTimezone, supportedTimezones } from '../../lib/userPreferences'
import { supportedLanguages } from '../../lib/languages'
import { buildZoneOptions } from '../../lib/timezoneOptions'

export const DEFAULT = ''

export const LANDING = {
  '': {
    label: 'preferences.landingDefault',
    hint: 'preferences.landingDefaultHint',
    icon: <GaugeGlassIcon size="1.4em" aria-hidden="true" />,
  },
  '/apps': {
    label: 'preferences.landingApps',
    hint: 'preferences.landingAppsHint',
    icon: <LayersGlassIcon size="1.4em" aria-hidden="true" />,
  },
  '/databases': {
    label: 'preferences.landingDatabases',
    hint: 'preferences.landingDatabasesHint',
    icon: <DatabaseGlassIcon size="1.4em" aria-hidden="true" />,
  },
  '/deployments': {
    label: 'preferences.landingDeployments',
    hint: 'preferences.landingDeploymentsHint',
    icon: <RocketGlassIcon size="1.4em" aria-hidden="true" />,
  },
  '/alerts': {
    label: 'preferences.landingAlerts',
    hint: 'preferences.landingAlertsHint',
    icon: <BellGlassIcon size="1.4em" aria-hidden="true" />,
  },
  '/settings/account': {
    label: 'preferences.landingAccount',
    hint: 'preferences.landingAccountHint',
    icon: <UserGlassIcon size="1.4em" aria-hidden="true" />,
  },
} as const

export function landingFor(
  path: string,
): (typeof LANDING)[keyof typeof LANDING] {
  return path in LANDING ? LANDING[path as keyof typeof LANDING] : LANDING['']
}

function englishLanguageName(code: string): string {
  try {
    return new Intl.DisplayNames(['en'], { type: 'language' }).of(code) ?? code
  } catch {
    return code
  }
}

/** useZoneOptions lists the browser default first, then every supported zone with its current offset. */
export function useZoneOptions(now: Date): SearchSelectOption[] {
  const { t } = useTranslation('account')
  return useMemo(
    () => [
      {
        value: DEFAULT,
        label: t('preferences.timezoneDefault', { zone: browserTimezone() }),
      },
      ...buildZoneOptions(supportedTimezones(), now),
    ],
    [now, t],
  )
}

/** useLanguageOptions lists "follow the browser" first, then each language with its English name. */
export function useLanguageOptions(): SearchSelectOption[] {
  const { t, i18n } = useTranslation('account')
  return useMemo(
    () => [
      { value: DEFAULT, label: t('preferences.languageDefault') },
      ...supportedLanguages(i18n).map((l) => {
        const english = englishLanguageName(l.code)
        return {
          value: l.code,
          label: l.name,
          trailing: english === l.name ? undefined : english,
          keywords: `${english} ${l.code}`,
        }
      }),
    ],
    [i18n, t],
  )
}
