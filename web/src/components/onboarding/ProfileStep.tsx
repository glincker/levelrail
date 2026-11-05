import { Suspense, useId, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { InfoGlassIcon } from '@thesvg/ui-react'
import { Input } from '@/components/ui/input'
import { SaveStatus, type SaveState } from '@/components/ui/save-status'
import { SettingRow } from '@/components/ui/setting-row'
import { SettingsSection } from '@/components/ui/settings-section'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { MAX_DISPLAY_NAME } from '../account/profileLimits'
import { UserAvatar } from '../account/UserAvatar'
import { useAutoSavePreferences } from '../account/useAutoSavePreferences'
import { useUpdateDisplayName } from '../../queries/myAccess'
import {
  preferencesQueryOptions,
  type Preferences,
} from '../../queries/accountSelf'
import { useSession } from '../../queries/security'

function GravatarRow({ saved }: { saved: Preferences }) {
  const { t } = useTranslation('onboarding')
  const { draft, patch } = useAutoSavePreferences(saved)
  const id = useId()
  return (
    <SettingRow
      title={<label htmlFor={id}>{t('profile.gravatar')}</label>}
      description={t('profile.gravatarHint')}
    >
      <Switch
        id={id}
        checked={draft.gravatar}
        onCheckedChange={(checked) => patch({ gravatar: checked })}
      />
    </SettingRow>
  )
}

function ProfileFields() {
  const { t } = useTranslation('onboarding')
  const { data: session } = useSession()
  const prefs = useQuery(preferencesQueryOptions())
  const update = useUpdateDisplayName()
  const [name, setName] = useState(session.display_name)
  const nameId = useId()

  // Saved on blur, so Continue right after typing still keeps the name.
  const commit = () => {
    const next = name.trim()
    if (next === '' || next === session.display_name) return
    update.mutate(next)
  }
  const state: SaveState = update.isPending
    ? 'saving'
    : update.isError
      ? 'error'
      : update.isSuccess
        ? 'saved'
        : 'idle'

  return (
    <SettingsSection
      title={t('profile.section')}
      actions={<SaveStatus state={state} />}
    >
      <SettingRow
        title={<label htmlFor={nameId}>{t('profile.name')}</label>}
        description={t('profile.nameHint', { max: MAX_DISPLAY_NAME })}
      >
        <Input
          id={nameId}
          value={name}
          maxLength={MAX_DISPLAY_NAME}
          autoComplete="name"
          aria-invalid={update.isError}
          onChange={(e) => setName(e.target.value)}
          onBlur={commit}
        />
      </SettingRow>
      <SettingRow
        title={t('profile.avatar')}
        description={
          <span className="inline-flex items-start gap-1">
            <InfoGlassIcon
              aria-hidden="true"
              className="mt-px size-4 shrink-0"
            />
            {t('profile.avatarHint')}
          </span>
        }
      >
        <UserAvatar
          name={name || session.display_name || session.username}
          seed={session.username}
          email={session.username}
          gravatar={prefs.data?.gravatar ?? false}
          size="lg"
        />
      </SettingRow>
      {prefs.data ? <GravatarRow saved={prefs.data} /> : null}
    </SettingsSection>
  )
}

/** ProfileStep edits the display name and the Gravatar opt-in; the avatar colour is derived from the account. */
export function ProfileStep() {
  return (
    <Suspense fallback={<Skeleton className="h-48 w-full" />}>
      <ProfileFields />
    </Suspense>
  )
}
