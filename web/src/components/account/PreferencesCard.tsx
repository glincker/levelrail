import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { SettingsCardSkeleton } from '../settings/SettingsSkeletons'
import { UserAvatar } from './UserAvatar'
import {
  preferencesQueryOptions,
  useSavePreferences,
  type Preferences,
} from '../../queries/accountSelf'
import { useSession } from '../../queries/security'
import { browserTimezone, supportedTimezones } from '../../lib/userPreferences'
import { supportedLanguages } from '../../lib/languages'

const DEFAULT = ''
const LANDING_KEYS = {
  '': 'preferences.landingDefault',
  '/apps': 'preferences.landingApps',
  '/databases': 'preferences.landingDatabases',
  '/deployments': 'preferences.landingDeployments',
  '/alerts': 'preferences.landingAlerts',
  '/settings/account': 'preferences.landingAccount',
} as const

type LandingPath = keyof typeof LANDING_KEYS

function isLanding(v: string): v is LandingPath {
  return v in LANDING_KEYS
}

function PreferencesForm({ saved }: { saved: Preferences }) {
  const { t, i18n } = useTranslation('account')
  const { data: session } = useSession()
  const save = useSavePreferences()
  const [draft, setDraft] = useState<Preferences>(saved)
  const zones = useMemo(() => supportedTimezones(), [])
  const languages = useMemo(() => supportedLanguages(i18n), [i18n])
  const dirty = JSON.stringify(draft) !== JSON.stringify(saved)
  const patch = (next: Partial<Preferences>) =>
    setDraft((d) => ({ ...d, ...next }))

  const submit = () => {
    save.mutate(draft, {
      onSuccess: () =>
        toast.add({ title: t('preferences.saved'), type: 'success' }),
      onError: (e) =>
        toast.add({
          title: t('preferences.saveFailed'),
          description: e.message,
          type: 'error',
        }),
    })
  }

  return (
    <form
      className="space-y-5"
      onSubmit={(e) => {
        e.preventDefault()
        if (dirty) submit()
      }}
    >
      <div className="flex items-center gap-4">
        <UserAvatar
          name={session.display_name || session.username}
          seed={session.username}
          email={session.username}
          gravatar={draft.gravatar}
          size="lg"
        />
        <div className="space-y-0.5 text-sm">
          <p className="font-medium">{t('preferences.avatarLabel')}</p>
          <p className="text-muted-foreground">{t('preferences.avatarHint')}</p>
        </div>
      </div>
      <FieldGroup>
        <Field>
          <FieldLabel htmlFor="pref-timezone">
            {t('preferences.timezone')}
          </FieldLabel>
          <Select
            value={draft.timezone}
            onValueChange={(v) => patch({ timezone: v ?? DEFAULT })}
          >
            <SelectTrigger id="pref-timezone" className="w-full max-w-md">
              <SelectValue>
                {(v: string) =>
                  v === DEFAULT
                    ? t('preferences.timezoneDefault', {
                        zone: browserTimezone(),
                      })
                    : v
                }
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={DEFAULT}>
                {t('preferences.timezoneDefault', { zone: browserTimezone() })}
              </SelectItem>
              {zones.map((z) => (
                <SelectItem key={z} value={z}>
                  {z}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <FieldDescription>{t('preferences.timezoneHint')}</FieldDescription>
        </Field>
        <Field>
          <FieldLabel htmlFor="pref-language">
            {t('preferences.language')}
          </FieldLabel>
          <Select
            value={draft.language}
            onValueChange={(v) => patch({ language: v ?? DEFAULT })}
          >
            <SelectTrigger id="pref-language" className="w-full max-w-md">
              <SelectValue>
                {(v: string) =>
                  v === DEFAULT
                    ? t('preferences.languageDefault')
                    : (languages.find((l) => l.code === v)?.name ?? v)
                }
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={DEFAULT}>
                {t('preferences.languageDefault')}
              </SelectItem>
              {languages.map((l) => (
                <SelectItem key={l.code} value={l.code}>
                  {l.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel htmlFor="pref-theme">{t('preferences.theme')}</FieldLabel>
          <Select
            value={draft.theme}
            onValueChange={(v) => {
              if (v === 'light' || v === 'dark' || v === 'system') {
                patch({ theme: v })
              } else {
                patch({ theme: '' })
              }
            }}
          >
            <SelectTrigger id="pref-theme" className="w-full max-w-md">
              <SelectValue>{(v: string) => themeLabel(v, t)}</SelectValue>
            </SelectTrigger>
            <SelectContent>
              {['', 'light', 'dark', 'system'].map((v) => (
                <SelectItem key={v || 'default'} value={v}>
                  {themeLabel(v, t)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel htmlFor="pref-landing">
            {t('preferences.landing')}
          </FieldLabel>
          <Select
            value={isLanding(draft.landing_page) ? draft.landing_page : ''}
            onValueChange={(v) => patch({ landing_page: v ?? DEFAULT })}
          >
            <SelectTrigger id="pref-landing" className="w-full max-w-md">
              <SelectValue>
                {(v: string) => t(LANDING_KEYS[isLanding(v) ? v : ''])}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              {(Object.keys(LANDING_KEYS) as LandingPath[]).map((path) => (
                <SelectItem key={path || 'default'} value={path}>
                  {t(LANDING_KEYS[path])}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field orientation="horizontal">
          <Switch
            id="pref-gravatar"
            checked={draft.gravatar}
            onCheckedChange={(checked) => patch({ gravatar: checked })}
          />
          <div className="space-y-1">
            <FieldLabel htmlFor="pref-gravatar">
              {t('preferences.gravatar')}
            </FieldLabel>
            <FieldDescription>{t('preferences.gravatarHint')}</FieldDescription>
          </div>
        </Field>
      </FieldGroup>
      <Button type="submit" disabled={!dirty || save.isPending} writeAction>
        {save.isPending ? t('preferences.saving') : t('preferences.save')}
      </Button>
    </form>
  )
}

function themeLabel(
  v: string,
  t: ReturnType<typeof useTranslation<'account'>>['t'],
): string {
  switch (v) {
    case 'light':
      return t('preferences.themeLight')
    case 'dark':
      return t('preferences.themeDark')
    case 'system':
      return t('preferences.themeSystem')
    default:
      return t('preferences.themeDefault')
  }
}

export function PreferencesCard() {
  const { t } = useTranslation('account')
  const prefs = useQuery(preferencesQueryOptions())

  if (prefs.isPending) return <SettingsCardSkeleton rows={4} />
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('preferences.title')}</CardTitle>
        <CardDescription>{t('preferences.description')}</CardDescription>
      </CardHeader>
      <CardContent>
        {prefs.isError ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.4em"
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
        ) : (
          <PreferencesForm
            key={JSON.stringify(prefs.data)}
            saved={prefs.data}
          />
        )}
      </CardContent>
    </Card>
  )
}
