import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { InfoGlassIcon, SettingsGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { SegmentedControl } from '@/components/ui/segmented-control'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { useGitSource } from '../../queries/gitSources'
import {
  siteChecksQueryOptions,
  useSiteSettings,
  useUpdateSiteSettings,
  type SiteSettings,
  type SiteSettingsPatch,
  type SiteSettingKey,
  type TrailingSlash,
} from '../../queries/pagesSites'

type Tri = 'auto' | 'on' | 'off'

const triOf = (v: boolean | null): Tri =>
  v === null ? 'auto' : v ? 'on' : 'off'

interface Form {
  spa_fallback: Tri
  clean_urls: Tri
  trailing_slash: TrailingSlash
  not_found_page: string
  build_command: string
  output_dir: string
}

function serverForm(s: SiteSettings): Form {
  return {
    spa_fallback: triOf(s.spa_fallback),
    clean_urls: triOf(s.clean_urls),
    trailing_slash: s.trailing_slash,
    not_found_page: s.not_found_page,
    build_command: s.build_command,
    output_dir: s.output_dir,
  }
}

function overridden(key: SiteSettingKey, f: Form): boolean {
  const v = f[key]
  return v !== 'auto' && v !== ''
}

interface RowProps {
  field: SiteSettingKey
  label: string
  hint?: string
  caption?: string
  resetLabel?: string
  dirty: boolean
  canReset: boolean
  busy: boolean
  onSave: () => void
  onReset: () => void
  children: ReactNode
}

function SettingRow({
  label,
  hint,
  caption,
  resetLabel,
  dirty,
  canReset,
  busy,
  onSave,
  onReset,
  children,
}: Readonly<RowProps>) {
  const { t } = useTranslation('pagesSites')
  return (
    <div className="grid items-center gap-x-3 gap-y-1.5 px-3 py-2.5 sm:grid-cols-[10rem_minmax(0,1fr)_auto]">
      <div className="flex items-center gap-1.5 text-sm font-medium">
        <span>{label}</span>
        {hint ? (
          <span
            tabIndex={0}
            role="img"
            title={hint}
            aria-label={hint}
            className="text-muted-foreground"
          >
            <InfoGlassIcon className="size-4" aria-hidden="true" />
          </span>
        ) : null}
      </div>
      <div className="min-w-0 space-y-1">
        {children}
        {caption ? (
          <p className="text-xs text-muted-foreground">{caption}</p>
        ) : null}
      </div>
      <div className="flex items-center gap-1.5 sm:justify-end">
        {dirty ? (
          <Button size="sm" disabled={busy} onClick={onSave}>
            {t('settings.save')}
          </Button>
        ) : null}
        {canReset ? (
          <Button size="sm" variant="ghost" disabled={busy} onClick={onReset}>
            {resetLabel ?? t('settings.reset')}
          </Button>
        ) : null}
      </div>
    </div>
  )
}

function SettingsFields({
  name,
  data,
}: Readonly<{ name: string; data: SiteSettings }>) {
  const { t } = useTranslation('pagesSites')
  const update = useUpdateSiteSettings(name)
  const checks = useQuery(siteChecksQueryOptions(name)).data
  const git = useGitSource(name).data
  const saved = serverForm(data)
  const [local, setLocal] = useState<Partial<Form>>({})
  const form: Form = { ...saved, ...local }
  const set = <K extends keyof Form>(k: K, v: Form[K]) =>
    setLocal((l) => ({ ...l, [k]: v }))
  const clear = (k: SiteSettingKey) =>
    setLocal((l) =>
      Object.fromEntries(Object.entries(l).filter(([key]) => key !== k)),
    )
  const done = (k: SiteSettingKey) => {
    clear(k)
    toast.add({ title: t('settings.saved'), type: 'success' })
  }
  const fail = (e: Error) => toast.add({ title: e.message, type: 'error' })
  const save = (k: SiteSettingKey) => {
    const v = form[k]
    const patch: SiteSettingsPatch = {}
    if (v === 'auto' || v === '') patch.reset = [k]
    else if (k === 'spa_fallback' || k === 'clean_urls')
      Object.assign(patch, { [k]: v === 'on' })
    else Object.assign(patch, { [k]: v })
    update.mutate(patch, { onSuccess: () => done(k), onError: fail })
  }
  const reset = (k: SiteSettingKey) =>
    update.mutate({ reset: [k] }, { onSuccess: () => done(k), onError: fail })
  const row = (k: SiteSettingKey) => ({
    field: k,
    dirty: form[k] !== saved[k],
    canReset: overridden(k, saved),
    busy: update.isPending,
    onSave: () => save(k),
    onReset: () => reset(k),
  })
  const state = (v: boolean | undefined) =>
    v === undefined
      ? undefined
      : t('settings.current', {
          value: v ? t('settings.on') : t('settings.off'),
        })
  const triOptions = [
    { value: 'auto' as const, label: t('settings.auto') },
    { value: 'on' as const, label: t('settings.on') },
    { value: 'off' as const, label: t('settings.off') },
  ]
  // Only when no per-site override exists does the git source value apply.
  const fromGit = (override: string, gitValue?: string) =>
    !override && gitValue
      ? t('settings.fromGit', { value: gitValue })
      : undefined
  const e = data.effective
  const notFoundCheck = checks?.checks.find((c) => c.id === 'custom_404')
  return (
    <div className="divide-y divide-border rounded-lg border border-border bg-card">
      <SettingRow
        {...row('spa_fallback')}
        label={t('settings.spa')}
        hint={t('settings.spaHint')}
        caption={state(e.spa)}
      >
        <SegmentedControl
          aria-label={t('settings.spa')}
          value={form.spa_fallback}
          options={triOptions}
          onValueChange={(v) => set('spa_fallback', v)}
        />
      </SettingRow>
      <SettingRow
        {...row('clean_urls')}
        label={t('settings.cleanUrls')}
        hint={t('settings.cleanUrlsHint')}
        caption={state(e.clean_urls)}
      >
        <SegmentedControl
          aria-label={t('settings.cleanUrls')}
          value={form.clean_urls}
          options={triOptions}
          onValueChange={(v) => set('clean_urls', v)}
        />
      </SettingRow>
      <SettingRow
        {...row('trailing_slash')}
        label={t('settings.trailingSlash')}
        caption={
          e.trailing_slash
            ? t('settings.current', { value: e.trailing_slash })
            : undefined
        }
      >
        <NativeSelect
          aria-label={t('settings.trailingSlash')}
          value={form.trailing_slash}
          className="w-full sm:w-56"
          onChange={(ev) =>
            set('trailing_slash', ev.target.value as TrailingSlash)
          }
        >
          <option value="">{t('settings.auto')}</option>
          <option value="off">{t('settings.slashOff')}</option>
          <option value="add">{t('settings.slashAdd')}</option>
          <option value="remove">{t('settings.slashRemove')}</option>
        </NativeSelect>
      </SettingRow>
      <SettingRow
        {...row('not_found_page')}
        label={t('settings.notFound')}
        caption={
          e.not_found
            ? t('settings.current', { value: e.not_found })
            : notFoundCheck?.title
        }
      >
        <Input
          aria-label={t('settings.notFound')}
          value={form.not_found_page}
          placeholder="/404.html"
          className="font-mono"
          onChange={(ev) => set('not_found_page', ev.target.value)}
        />
      </SettingRow>
      <SettingRow
        {...row('build_command')}
        label={t('settings.build')}
        hint={t('settings.gitOnlyHint')}
        resetLabel={git ? t('settings.resetGit') : undefined}
        caption={fromGit(saved.build_command, git?.build_command)}
      >
        <Input
          aria-label={t('settings.build')}
          value={form.build_command}
          placeholder={t('settings.buildPlaceholder')}
          className="font-mono"
          onChange={(ev) => set('build_command', ev.target.value)}
        />
      </SettingRow>
      <SettingRow
        {...row('output_dir')}
        label={t('settings.output')}
        hint={t('settings.gitOnlyHint')}
        resetLabel={git ? t('settings.resetGit') : undefined}
        caption={fromGit(saved.output_dir, git?.publish_dir)}
      >
        <Input
          aria-label={t('settings.output')}
          value={form.output_dir}
          placeholder={t('settings.outputPlaceholder')}
          className="font-mono"
          onChange={(ev) => set('output_dir', ev.target.value)}
        />
      </SettingRow>
      {update.isError ? (
        <Alert variant="destructive" role="alert" className="rounded-none">
          <AlertDescription>{update.error.message}</AlertDescription>
        </Alert>
      ) : null}
    </div>
  )
}

/** Serving and build overrides, one compact row each; Auto leaves the decision to app.yaml. */
export function SiteSettingsForm({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('pagesSites')
  const q = useSiteSettings(name)
  if (q.isPending) return <Skeleton className="h-48 w-full" />
  if (q.isError) {
    return (
      <EmptyState
        compact
        icon={<SettingsGlassIcon className="size-6" />}
        title={t('settings.loadFailed')}
        action={
          <Button size="sm" variant="outline" onClick={() => void q.refetch()}>
            {t('routes.retry')}
          </Button>
        }
      />
    )
  }
  return <SettingsFields name={name} data={q.data} />
}

export default SiteSettingsForm
