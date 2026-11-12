import { useId, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { SaveStatus } from '@/components/ui/save-status'
import { SearchSelect } from '@/components/ui/search-select'
import { SettingRow } from '@/components/ui/setting-row'
import { SettingsSection } from '@/components/ui/settings-section'
import { Skeleton } from '@/components/ui/skeleton'
import { LandingSelect } from '../account/LandingSelect'
import { useZoneOptions } from '../account/preferenceOptions'
import { ThemeChoice, type ThemeValue } from '../account/ThemeChoice'
import { LanguageSelect } from '../language/LanguageSelect'
import { useAutoSavePreferences } from '../account/useAutoSavePreferences'
import { ChoiceGroup, type Choice } from '../settings/ChoiceGroup'
import { AppearanceLinkCard } from '../settings/AppearanceLinkCard'
import { setDensity, useDensity, type Density } from '../../lib/density'
import {
  preferencesQueryOptions,
  type Preferences,
} from '../../queries/accountSelf'

function PreferenceRows({ saved }: { saved: Preferences }) {
  const { t } = useTranslation('onboarding')
  const { t: ta } = useTranslation('account')
  const { t: ts } = useTranslation('settings')
  const { draft, patch, save } = useAutoSavePreferences(saved)
  const density = useDensity()
  const [now, setNow] = useState(() => new Date())
  const themeLabelId = useId()
  const zoneId = useId()
  const languageId = useId()
  const landingId = useId()
  const zoneOptions = useZoneOptions(now)

  const densityOptions: Choice<Density>[] = (
    ['compact', 'comfortable'] as const
  ).map((v) => ({
    value: v,
    label: ts(`generalPage.density.${v}`),
    hint: ts(`appearancePage.density.${v}Hint`),
    isDefault: v === 'compact',
    isRecommended: v === 'compact',
  }))
  const themeValue: ThemeValue = draft.theme === '' ? 'system' : draft.theme

  return (
    <SettingsSection
      title={t('preferences.section')}
      actions={
        <SaveStatus
          state={save.isPending ? 'saving' : save.isSuccess ? 'saved' : 'idle'}
        />
      }
    >
      <SettingRow
        title={<span id={themeLabelId}>{ta('preferences.theme')}</span>}
      >
        <ThemeChoice
          value={themeValue}
          labelledBy={themeLabelId}
          onChange={(v) => patch({ theme: v })}
        />
      </SettingRow>
      <SettingRow
        title={ts('generalPage.density.title')}
        description={ts('appearancePage.density.description')}
      >
        <ChoiceGroup
          label={ts('generalPage.density.title')}
          value={density}
          options={densityOptions}
          onChange={setDensity}
          className="sm:grid-cols-2"
          defaultLabel={ts('appearancePage.defaultBadge')}
          recommendedLabel={ts('appearancePage.recommendedBadge')}
        />
      </SettingRow>
      <SettingRow
        title={<label htmlFor={languageId}>{ta('preferences.language')}</label>}
      >
        <LanguageSelect
          id={languageId}
          allowDefault
          value={draft.language}
          onChange={(v) => patch({ language: v })}
        />
      </SettingRow>
      <SettingRow
        title={<label htmlFor={zoneId}>{ta('preferences.timezone')}</label>}
      >
        <SearchSelect
          id={zoneId}
          options={zoneOptions}
          value={draft.timezone}
          onValueChange={(v) => patch({ timezone: v })}
          groupLabels={{
            common: ta('preferences.zonesCommon'),
            all: ta('preferences.zonesAll'),
          }}
          searchPlaceholder={ta('preferences.zoneSearch')}
          emptyMessage={ta('preferences.zoneEmpty')}
          onOpenChange={(open) => {
            if (open) setNow(new Date())
          }}
        />
      </SettingRow>
      <SettingRow
        title={<label htmlFor={landingId}>{ta('preferences.landing')}</label>}
      >
        <LandingSelect
          id={landingId}
          value={draft.landing_page}
          onChange={(v) => patch({ landing_page: v })}
        />
      </SettingRow>
    </SettingsSection>
  )
}

/** PreferencesStep is the compact version of Settings, Appearance and Account preferences; every change saves at once. */
export function PreferencesStep() {
  const prefs = useQuery(preferencesQueryOptions())
  return (
    <div className="space-y-4">
      {prefs.data ? (
        <PreferenceRows saved={prefs.data} />
      ) : (
        <Skeleton className="h-64 w-full" />
      )}
      <AppearanceLinkCard />
    </div>
  )
}
