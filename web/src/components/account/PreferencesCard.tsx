import { useId, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  BellGlassIcon,
  DatabaseGlassIcon,
  GaugeGlassIcon,
  InfoGlassIcon,
  LayersGlassIcon,
  RocketGlassIcon,
  RotateCcwGlassIcon,
  TriangleAlertGlassIcon,
  UserGlassIcon,
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
import { SearchSelect } from '@/components/ui/search-select'
import { SettingRow, SettingRows } from '@/components/ui/setting-row'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { SettingsCardSkeleton } from '../settings/SettingsSkeletons'
import { useTheme } from '../ThemeProvider'
import { DEFAULT_PREFERENCES } from './preferenceDefaults'
import { ThemeChoice, type ThemeValue } from './ThemeChoice'
import {
  preferencesQueryOptions,
  useSavePreferences,
  type Preferences,
} from '../../queries/accountSelf'
import { browserTimezone, supportedTimezones } from '../../lib/userPreferences'
import { supportedLanguages } from '../../lib/languages'
import { buildZoneOptions } from '../../lib/timezoneOptions'

const DEFAULT = ''

const LANDING = {
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

function landingFor(path: string): (typeof LANDING)[keyof typeof LANDING] {
  return path in LANDING ? LANDING[path as keyof typeof LANDING] : LANDING['']
}

function englishLanguageName(code: string): string {
  try {
    return new Intl.DisplayNames(['en'], { type: 'language' }).of(code) ?? code
  } catch {
    return code
  }
}

function PreferencesForm({ saved }: { saved: Preferences }) {
  const { t, i18n } = useTranslation('account')
  const save = useSavePreferences()
  const { setTheme } = useTheme()
  const [draft, setDraft] = useState<Preferences>(saved)
  const [now, setNow] = useState(() => new Date())
  const themeLabelId = useId()
  const zoneId = useId()
  const languageId = useId()
  const landingId = useId()
  const gravatarId = useId()

  const zoneOptions = useMemo(
    () => [
      {
        value: DEFAULT,
        label: t('preferences.timezoneDefault', { zone: browserTimezone() }),
      },
      ...buildZoneOptions(supportedTimezones(), now),
    ],
    [now, t],
  )
  const languageOptions = useMemo(
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

  // Each change saves at once; the theme is also applied before the round trip.
  const apply = (next: Preferences) => {
    setDraft(next)
    if (next.theme !== draft.theme && next.theme !== '') setTheme(next.theme)
    save.mutate(next, {
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
          <span
            role="status"
            aria-live="polite"
            className="text-xs text-muted-foreground"
          >
            {save.isPending
              ? t('preferences.saving')
              : save.isSuccess
                ? t('preferences.saved')
                : ''}
          </span>
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
            <SearchSelect
              id={languageId}
              options={languageOptions}
              value={draft.language}
              onValueChange={(v) => patch({ language: v })}
              searchPlaceholder={t('preferences.languageSearch')}
              emptyMessage={t('preferences.languageEmpty')}
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
            <Select
              value={draft.landing_page in LANDING ? draft.landing_page : ''}
            >
              <SelectTrigger id={landingId} className="h-9 w-full">
                <SelectValue>
                  {(v: string) => {
                    const item = landingFor(v)
                    return (
                      <span className="flex items-center gap-2">
                        {item.icon}
                        {t(item.label)}
                      </span>
                    )
                  }}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {Object.entries(LANDING).map(([path, item]) => (
                  <SelectItem key={path || 'default'} value={path}>
                    <span className="flex items-center gap-2">
                      {item.icon}
                      <span className="flex flex-col">
                        <span>{t(item.label)}</span>
                        <span className="text-xs text-muted-foreground">
                          {t(item.hint)}
                        </span>
                      </span>
                    </span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
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
