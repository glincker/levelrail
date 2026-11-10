import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { HammerGlassIcon } from '@thesvg/ui-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import { toast } from '@/components/ui/toast'
import { useBuildSettings, useSaveBuildSettings } from './queries'
import {
  BUILDERS,
  draftFrom,
  savePayload,
  validateDraft,
  type BuildSettings,
} from './buildSettingsForm'

export function BuildSettingsCard({ appName }: { appName: string }) {
  const q = useBuildSettings(appName)
  if (!q.data) return null
  return (
    <BuildSettingsForm
      key={q.data.updated_at ?? 'new'}
      appName={appName}
      settings={q.data}
    />
  )
}

function BuildSettingsForm({
  appName,
  settings,
}: {
  appName: string
  settings: BuildSettings
}) {
  const { t } = useTranslation('gitSource')
  const save = useSaveBuildSettings(appName)
  const [draft, setDraft] = useState(() => draftFrom(settings))
  const errors = validateDraft(draft, settings)
  const payload = savePayload(draft, settings)
  const dirty = Object.keys(payload).length > 0
  const invalid = Object.keys(errors).length > 0
  const err = (k: keyof typeof errors) =>
    errors[k] ? (
      <p className="text-xs text-destructive">
        {t(`buildSettings.errors.${errors[k]}`)}
      </p>
    ) : null

  const submit = () =>
    save.mutate(payload, {
      onSuccess: () =>
        toast.add({ title: t('buildSettings.saved'), type: 'success' }),
      onError: (e) =>
        toast.add({
          title: t('buildSettings.failed'),
          description: e.message,
          type: 'error',
        }),
    })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <HammerGlassIcon className="size-5 text-muted-foreground" />
          {t('buildSettings.title')}
        </CardTitle>
        <CardDescription>{t('buildSettings.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="grid gap-3 sm:grid-cols-2">
          <div className="space-y-1.5">
            <Label htmlFor="bs-builder">{t('buildSettings.builder')}</Label>
            <NativeSelect
              id="bs-builder"
              className="w-full"
              value={draft.builder}
              disabled={!settings.has_git_source}
              onChange={(e) => setDraft({ ...draft, builder: e.target.value })}
            >
              {BUILDERS.map((b) => (
                <option key={b} value={b}>
                  {b}
                </option>
              ))}
            </NativeSelect>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="bs-dockerfile">
              {t('buildSettings.dockerfile')}
            </Label>
            <Input
              id="bs-dockerfile"
              value={draft.dockerfile_path}
              disabled={!settings.has_git_source}
              placeholder={t('buildSettings.dockerfilePlaceholder')}
              spellCheck={false}
              className="font-mono text-xs"
              onChange={(e) =>
                setDraft({ ...draft, dockerfile_path: e.target.value })
              }
            />
            {err('dockerfile_path')}
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="bs-target">{t('buildSettings.target')}</Label>
            <Input
              id="bs-target"
              value={draft.target}
              placeholder={t('buildSettings.targetPlaceholder')}
              spellCheck={false}
              className="font-mono text-xs"
              onChange={(e) => setDraft({ ...draft, target: e.target.value })}
            />
            {err('target')}
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="bs-provider">{t('buildSettings.provider')}</Label>
            <NativeSelect
              id="bs-provider"
              className="w-full"
              value={draft.provider}
              onChange={(e) => setDraft({ ...draft, provider: e.target.value })}
            >
              <option value="">{t('buildSettings.providerAuto')}</option>
              {settings.providers.map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </NativeSelect>
            {err('provider')}
          </div>
        </div>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            className="size-4 accent-primary"
            checked={draft.no_cache}
            onChange={(e) => setDraft({ ...draft, no_cache: e.target.checked })}
          />
          {t('buildSettings.noCache')}
        </label>
        {!settings.has_git_source && (
          <p className="text-xs text-muted-foreground">
            {t('buildSettings.needsGit')}
          </p>
        )}
        <Button
          size="sm"
          onClick={submit}
          disabled={!dirty || invalid || save.isPending}
        >
          {save.isPending ? t('buildSettings.saving') : t('buildSettings.save')}
        </Button>
      </CardContent>
    </Card>
  )
}
