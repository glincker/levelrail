import { useId, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  InfoGlassIcon,
  RotateCcwGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { SaveStatus } from '@/components/ui/save-status'
import { SearchSelect } from '@/components/ui/search-select'
import { SettingRow, SettingRows } from '@/components/ui/setting-row'
import { Switch } from '@/components/ui/switch'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { SettingsCardSkeleton } from '../settings/SettingsSkeletons'
import { DEFAULT_PREFERENCES } from './preferenceDefaults'
import { LanguageSelect } from '../language/LanguageSelect'
import { LandingSelect } from './LandingSelect'
import { useZoneOptions } from './preferenceOptions'
import { ThemeChoice, type ThemeValue } from './ThemeChoice'
import { useAutoSavePreferences } from './useAutoSavePreferences'
import {
  preferencesQueryOptions,
  type Preferences,
} from '../../queries/accountSelf'

function PreferencesForm({ saved }: { saved: Preferences }) {
  const { t } = useTranslation('account')
  const { draft, apply, patch, save } = useAutoSavePreferences(saved)
  const [now, setNow] = useState(() => new Date())
  const themeLabelId = useId()
  const zoneId = useId()
  const languageId = useId()
  const landingId = useId()
  const gravatarId = useId()
  const zoneOptions = useZoneOptions(now)
  const isDefault =
    JSON.stringify(draft) === JSON.stringify(DEFAULT_PREFERENCES)

  const themeValue: ThemeValue = draft.theme === '' ? 'system' : draft.theme
  const rowCls = 'py-3 xl:grid-cols-[minmax(0,1fr)_20rem]'

  return (
    <>
      <CardHeader>
        <CardTitle>{t('preferences.title')}</CardTitle>
        <CardDescription>{t('preferences.description')}</CardDescription>
        <CardAction className="flex items-center gap-2">
          <SaveStatus
            state={
              save.isPending ? 'saving' : save.isSuccess ? 'saved' : 'idle'
            }
          />
          <Button
            type="button"
            size="sm"
            variant="ghost"
            disabled={isDefault || save.isPending}
            onClick={() => apply(DEFAULT_PREFERENCES)}
          >
            <RotateCcwGlassIcon size="1.4em" aria-hidden="true" />
            {t('preferences.reset')}
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent>
        <SettingRows>
          <SettingRow
            className={rowCls}
            title={<label htmlFor={zoneId}>{t('preferences.timezone')}</label>}
            description={t('preferences.timezoneHint')}
          >
            <SearchSelect
              id={zoneId}
              options={zoneOptions}
              value={draft.timezone}
              onValueChange={(v) => patch({ timezone: v })}
              groupLabels={{
                common: t('preferences.zonesCommon'),
                all: t('preferences.zonesAll'),
              }}
              searchPlaceholder={t('preferences.zoneSearch')}
              emptyMessage={t('preferences.zoneEmpty')}
              onOpenChange={(open) => {
                if (open) setNow(new Date())
              }}
            />
          </SettingRow>
          <SettingRow
            className={rowCls}
            title={
              <label htmlFor={languageId}>{t('preferences.language')}</label>
            }
            description={t('preferences.languageHint')}
          >
            <LanguageSelect
              id={languageId}
              allowDefault
              value={draft.language}
              onChange={(v) => patch({ language: v })}
            />
          </SettingRow>
          <SettingRow
            className={rowCls}
            title={<span id={themeLabelId}>{t('preferences.theme')}</span>}
            description={t('preferences.themeHint')}
          >
            <ThemeChoice
              value={themeValue}
              labelledBy={themeLabelId}
              onChange={(v) => patch({ theme: v })}
            />
          </SettingRow>
          <SettingRow
            className={rowCls}
            title={
              <label htmlFor={landingId}>{t('preferences.landing')}</label>
            }
            description={t('preferences.landingHint')}
          >
            <LandingSelect
              id={landingId}
              value={draft.landing_page}
              onChange={(v) => patch({ landing_page: v })}
            />
          </SettingRow>
          <SettingRow
            className={rowCls}
            title={
              <label htmlFor={gravatarId}>{t('preferences.gravatar')}</label>
            }
            description={
              <span className="inline-flex items-center gap-1">
                {t('preferences.gravatarShort')}
                <Tooltip>
                  <TooltipTrigger
                    type="button"
                    aria-label={t('preferences.gravatarMore')}
                    className="rounded-sm text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    <InfoGlassIcon size="1.4em" aria-hidden="true" />
                  </TooltipTrigger>
                  <TooltipContent>
                    {t('preferences.gravatarHint')}
                  </TooltipContent>
                </Tooltip>
              </span>
            }
          >
            <Switch
              id={gravatarId}
              checked={draft.gravatar}
              onCheckedChange={(checked) => patch({ gravatar: checked })}
            />
          </SettingRow>
        </SettingRows>
      </CardContent>
    </>
  )
}

export function PreferencesCard() {
  const { t } = useTranslation('account')
  const prefs = useQuery(preferencesQueryOptions())

  if (prefs.isPending) return <SettingsCardSkeleton rows={4} />
  if (prefs.isError) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>{t('preferences.title')}</CardTitle>
        </CardHeader>
        <CardContent>
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.7em"
            />
            <AlertDescription className="flex items-center justify-between gap-3">
              <span>{t('common.loadFailed')}</span>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => void prefs.refetch()}
              >
                {t('common.retry')}
              </Button>
            </AlertDescription>
          </Alert>
        </CardContent>
      </Card>
    )
  }
  return (
    <Card>
      <PreferencesForm saved={prefs.data} />
    </Card>
  )
}
