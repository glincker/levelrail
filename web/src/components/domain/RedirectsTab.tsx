import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  usePolicyPreview,
  useSaveRedirectSettings,
  useSetAliases,
  useSetCanonical,
  useDomainRedirects,
} from '../../queries/domainPolicies'
import type {
  CanonicalPreset,
  DomainRedirects,
  RedirectSettings,
} from '../../queries/domainPolicyTypes'
import { HopsStrip } from './HopsStrip'
import {
  CheckboxField,
  FieldMessage,
  NativeSelect,
  SaveBar,
  Section,
} from './PolicyShared'
import { useDebounced, usePolicyDraft } from './usePolicyDraft'

const STATUSES = ['301', '302', '307', '308']

export function RedirectsTab({ app, domain }: { app: string; domain: string }) {
  const { t } = useTranslation('domainPolicies')
  const { data, error, isLoading, refetch } = useDomainRedirects(app, domain)
  if (isLoading) return <Skeleton className="h-64 w-full" />
  if (error || !data) {
    return (
      <Alert variant="destructive">
        <AlertDescription>
          {error?.message ?? t('page.loadFailed')}{' '}
          <Button
            type="button"
            size="xs"
            variant="link"
            onClick={() => void refetch()}
          >
            {t('page.retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  }
  return <RedirectsEditor app={app} domain={domain} data={data} />
}

function RedirectsEditor({
  app,
  domain,
  data,
}: {
  app: string
  domain: string
  data: DomainRedirects
}) {
  const { t } = useTranslation('domainPolicies')
  const draft = usePolicyDraft<RedirectSettings>(
    app,
    domain,
    'redirects',
    data.settings,
  )
  const s = draft.draft
  const set = (patch: Partial<RedirectSettings>) =>
    draft.setDraft({ ...s, ...patch })
  const saveSettings = useSaveRedirectSettings(app, domain)
  const [preset, setPreset] = useState<CanonicalPreset>(data.canonical.preset)
  const [canonicalStatus, setCanonicalStatus] = useState('301')
  const [replace, setReplace] = useState(false)
  const canonical = useSetCanonical(app, domain)

  const [primary, setPrimary] = useState(domain)
  const initialAliases = useMemo(
    () =>
      data.app_domains
        .filter(
          (d) =>
            d.redirect?.target_url.replace(/^https?:\/\//, '').split('/')[0] ===
            domain,
        )
        .map((d) => d.domain),
    [data, domain],
  )
  const [aliases, setAliases] = useState<string[]>(initialAliases)
  const [aliasStatus, setAliasStatus] = useState('301')
  const saveAliases = useSetAliases(app, primary)

  const debounced = useDebounced(s)
  const previewInput = useMemo(
    () => ({
      policy: { redirects: debounced },
      urls: data.samples.map((x) => x.url),
      canonical_preset: preset !== data.canonical.preset ? preset : undefined,
    }),
    [debounced, data, preset],
  )
  const preview = usePolicyPreview(app, domain, previewInput)
  const samples = preview.data?.hops ?? data.samples
  const handled = data.effective.handled_by

  return (
    <div className="space-y-4">
      <Section
        title={t('redirects.howTitle')}
        description={t('redirects.howHelp')}
      >
        <HopsStrip samples={samples} />
      </Section>

      <Section title={t('redirects.forceTitle')}>
        <div className="flex flex-wrap items-center gap-2 text-sm">
          <Badge variant={handled === 'none' ? 'muted' : 'success'}>
            {handled === 'proxy'
              ? t('redirects.handledProxy')
              : handled === 'here'
                ? t('redirects.handledHere')
                : t('redirects.handledNone')}
          </Badge>
          <span className="text-xs text-muted-foreground">
            {data.effective.explanation}
          </span>
        </div>
        <CheckboxField
          id="force-https"
          label={t('redirects.forceHttps')}
          checked={s.force_https}
          onChange={(v) => set({ force_https: v })}
        />
        <div className="flex flex-wrap gap-2">
          <NativeSelect
            id="force-status"
            label={t('redirects.forceStatus')}
            value={String(s.force_https_status ?? 308)}
            options={[
              { value: '308', label: t('redirects.status308') },
              { value: '301', label: t('redirects.status301') },
            ]}
            onChange={(v) => set({ force_https_status: Number(v) })}
          />
          <NativeSelect
            id="trailing-slash"
            label={t('redirects.trailingSlash')}
            value={s.trailing_slash || 'off'}
            options={[
              { value: 'off', label: t('redirects.slashOff') },
              { value: 'add', label: t('redirects.slashAdd') },
              { value: 'remove', label: t('redirects.slashRemove') },
            ]}
            onChange={(v) =>
              set({ trailing_slash: v as RedirectSettings['trailing_slash'] })
            }
          />
        </div>
        <CheckboxField
          id="lowercase-host"
          label={t('redirects.lowercase')}
          checked={Boolean(s.lowercase_host)}
          onChange={(v) => set({ lowercase_host: v })}
        />
        <SaveBar
          saved={data.settings}
          draft={s}
          dirty={draft.dirty}
          saving={saveSettings.isPending}
          error={saveSettings.error?.message}
          onSave={() => saveSettings.mutate(s)}
          onDiscard={draft.discard}
        />
      </Section>

      <Section
        title={t('redirects.canonicalTitle')}
        description={t('redirects.canonicalHelp')}
      >
        {data.canonical.unavailable ? (
          <p className="text-sm text-muted-foreground">
            {data.canonical.unavailable}
          </p>
        ) : (
          <fieldset className="space-y-2">
            <legend className="sr-only">{t('redirects.canonicalTitle')}</legend>
            {(['www-to-apex', 'apex-to-www', 'both'] as const).map((p) => (
              <label key={p} className="flex items-center gap-2 text-sm">
                <input
                  type="radio"
                  name="canonical"
                  value={p}
                  className="accent-primary"
                  checked={preset === p}
                  onChange={() => setPreset(p)}
                />
                {t(`redirects.preset.${p}`)}
              </label>
            ))}
            {!data.canonical.counterpart_attached &&
            data.canonical.counterpart ? (
              <p className="text-xs text-muted-foreground">
                {data.canonical.counterpart_app
                  ? t('redirects.counterpartElsewhere', {
                      host: data.canonical.counterpart,
                      app: data.canonical.counterpart_app,
                    })
                  : t('redirects.counterpartMissing', {
                      host: data.canonical.counterpart,
                    })}{' '}
                {data.canonical.dns_hint}
              </p>
            ) : null}
            <div className="flex flex-wrap items-end gap-2">
              <NativeSelect
                id="canonical-status"
                label={t('redirects.status')}
                value={canonicalStatus}
                options={STATUSES.map((v) => ({ value: v, label: v }))}
                onChange={setCanonicalStatus}
              />
              <CheckboxField
                id="canonical-replace"
                label={t('redirects.replace')}
                checked={replace}
                onChange={setReplace}
              />
              <Button
                type="button"
                size="sm"
                disabled={canonical.isPending}
                onClick={() =>
                  canonical.mutate({
                    preset,
                    status_code: Number(canonicalStatus),
                    replace,
                  })
                }
              >
                {t('redirects.applyPreset')}
              </Button>
            </div>
            <FieldMessage message={canonical.error?.message} />
          </fieldset>
        )}
      </Section>

      <Section
        title={t('redirects.aliasesTitle')}
        description={t('redirects.aliasesHelp')}
      >
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-xs text-muted-foreground">
              <th className="py-1 font-normal">{t('redirects.domain')}</th>
              <th className="py-1 font-normal">{t('redirects.primary')}</th>
              <th className="py-1 font-normal">{t('redirects.alias')}</th>
              <th className="py-1 font-normal">{t('redirects.current')}</th>
            </tr>
          </thead>
          <tbody>
            {data.app_domains.map((d) => (
              <tr key={d.domain} className="border-t border-border">
                <td className="py-1 font-mono text-xs">{d.domain}</td>
                <td className="py-1">
                  <input
                    type="radio"
                    name="primary"
                    className="accent-primary"
                    aria-label={t('redirects.makePrimary', {
                      domain: d.domain,
                    })}
                    checked={primary === d.domain}
                    disabled={d.wildcard}
                    onChange={() => {
                      setPrimary(d.domain)
                      setAliases((a) => a.filter((x) => x !== d.domain))
                    }}
                  />
                </td>
                <td className="py-1">
                  <input
                    type="checkbox"
                    className="accent-primary"
                    aria-label={t('redirects.makeAlias', { domain: d.domain })}
                    disabled={d.domain === primary || d.wildcard}
                    checked={aliases.includes(d.domain)}
                    onChange={(e) =>
                      setAliases((a) =>
                        e.target.checked
                          ? [...a, d.domain]
                          : a.filter((x) => x !== d.domain),
                      )
                    }
                  />
                </td>
                <td className="py-1 text-xs text-muted-foreground">
                  {d.maintenance
                    ? t('redirects.maintenance')
                    : d.redirect
                      ? `${d.redirect.status_code} ${d.redirect.target_url}`
                      : t('redirects.servesApp')}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <div className="flex flex-wrap items-end gap-2">
          <NativeSelect
            id="alias-status"
            label={t('redirects.status')}
            value={aliasStatus}
            options={STATUSES.map((v) => ({ value: v, label: v }))}
            onChange={setAliasStatus}
          />
          <Button
            type="button"
            size="sm"
            disabled={saveAliases.isPending}
            onClick={() =>
              saveAliases.mutate({ aliases, status_code: Number(aliasStatus) })
            }
          >
            {t('redirects.saveAliases')}
          </Button>
        </div>
        <FieldMessage message={saveAliases.error?.message} />
      </Section>
      {(data.warnings ?? []).map((w) => (
        <Alert key={w}>
          <AlertDescription>{w}</AlertDescription>
        </Alert>
      ))}
    </div>
  )
}
