import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import { CheckGlassIcon, LanguagesGlassIcon } from '@thesvg/ui-react'
import { toast } from '@/components/ui/toast'
import type { PaletteItem } from '../components/commandPaletteData'
import {
  preferencesQueryOptions,
  useSavePreferences,
} from '../queries/accountSelf'
import {
  browserLanguageOffer,
  paletteLanguages,
  type PaletteLanguage,
} from '../lib/paletteLanguage'

export const LANGUAGE_GROUP = 'Languages'

export interface PaletteLanguageItems {
  /** Entries searchable from the top level: the step opener, one per language, the browser shortcut. */
  base: PaletteItem[]
  /** The second step: every language, current one ticked. */
  step: PaletteItem[]
}

// Applies the language at once through i18n, then saves it like Settings does so it survives a reload.
export function usePaletteLanguage(openStep: () => void): PaletteLanguageItems {
  const { t, i18n } = useTranslation('palette')
  const qc = useQueryClient()
  const save = useSavePreferences().mutate

  const apply = React.useCallback(
    (lang: PaletteLanguage) => {
      void i18n.changeLanguage(lang.code)
      void qc
        .fetchQuery(preferencesQueryOptions())
        .then((prefs) => {
          save(
            { ...prefs, language: lang.code },
            {
              onError: (e) =>
                toast.add({
                  title: t('language.saveFailed'),
                  description: e.message,
                  type: 'error',
                }),
            },
          )
        })
        .catch(() => undefined)
    },
    [i18n, qc, save, t],
  )

  const activeLanguage = i18n?.language
  return React.useMemo<PaletteLanguageItems>(() => {
    // Tests and partial providers can render without an i18n instance.
    if (!i18n) return { base: [], step: [] }
    void activeLanguage
    const langs = paletteLanguages(i18n)
    const aliases = t('language.keywords')
    const icon = <LanguagesGlassIcon size="1.4em" />
    const row = (l: PaletteLanguage, key: string): PaletteItem => ({
      key,
      label: t('language.switchTo', { name: l.name }),
      group: LANGUAGE_GROUP,
      icon,
      keywords: `${aliases} ${l.name} ${l.englishName} ${l.code}`,
      badge: l.current ? (
        <CheckGlassIcon
          className="size-5 text-primary"
          aria-label={t('language.current')}
        />
      ) : undefined,
      run: () => apply(l),
    })
    const opener: PaletteItem = {
      key: 'action-change-language',
      label: t('language.change'),
      keywords: aliases,
      group: 'Actions',
      icon,
      keepOpen: true,
      run: openStep,
      preview: { kind: 'action', description: t('language.description') },
    }
    const offer = browserLanguageOffer(i18n, navigator.languages ?? [])
    const browser = offer
      ? [
          {
            ...row(offer, 'action-language-browser'),
            label: t('language.browser', { name: offer.name }),
            group: 'Actions',
          },
        ]
      : []
    return {
      base: [
        opener,
        ...browser,
        ...langs
          .filter((l) => l.code !== offer?.code)
          .map((l) => row(l, `language-${l.code}`)),
      ],
      step: langs.map((l) => ({
        ...row(l, `language-step-${l.code}`),
        label: l.name,
        subtitle: l.englishName,
      })),
    }
  }, [i18n, activeLanguage, t, apply, openStep])
}
