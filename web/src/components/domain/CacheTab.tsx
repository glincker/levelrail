import { useTranslation } from 'react-i18next'
import { PlusGlassIcon, TrashGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useApplyCachePreset } from '../../queries/cdnCache'
import { CacheStatsCard } from './CacheStatsCard'
import { PreviewControls } from './PreviewControls'
import {
  CheckboxField,
  FieldMessage,
  NativeSelect,
  SaveBar,
  Section,
} from './PolicyShared'
import { withNextjsPreset } from '../../lib/cachePresets'
import { splitList } from './policyUtils'
import { usePolicyEditor } from './usePolicyEditor'
import type {
  CacheRule,
  CacheSpec,
  DomainPolicies,
} from '../../queries/domainPolicyTypes'

const EMPTY: CacheSpec = { enabled: false, rules: [] }

export function CacheTab({
  app,
  domain,
  policies,
}: {
  app: string
  domain: string
  policies: DomainPolicies
}) {
  const { t } = useTranslation('domainPolicies')
  const saved = policies.policy.cache ?? EMPTY
  const ed = usePolicyEditor(app, domain, 'cache', saved)
  const c = ed.draft
  const preset = useApplyCachePreset(app)
  const setRule = (i: number, patch: Partial<CacheRule>) =>
    ed.setDraft({
      ...c,
      rules: c.rules.map((r, j) => (j === i ? { ...r, ...patch } : r)),
    })

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_20rem]">
      <div className="space-y-4">
        <Section title={t('cache.title')} description={t('cache.help')}>
          <CheckboxField
            id="cache-enabled"
            label={t('cache.enabled')}
            checked={c.enabled}
            onChange={(v) => ed.setDraft({ ...c, enabled: v })}
          />
          <div className="flex flex-wrap items-center gap-3 rounded-lg bg-muted/40 px-4 py-3">
            <p className="min-w-48 flex-1 text-sm text-muted-foreground">
              {t('cache.presetHelp')}
            </p>
            <Button
              type="button"
              size="sm"
              variant="outline"
              writeAction
              disabled={preset.isPending || ed.dirty}
              onClick={() => preset.mutate(domain)}
            >
              {t('cache.preset')}
            </Button>
          </div>
          <div className="flex flex-wrap items-center gap-3 rounded-lg bg-muted/40 px-4 py-3">
            <p className="min-w-48 flex-1 text-sm text-muted-foreground">
              {t('cache.nextjsPresetHelp')}
            </p>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => ed.setDraft(withNextjsPreset(c))}
            >
              {t('cache.nextjsPreset')}
            </Button>
          </div>
          <FieldMessage message={preset.error?.message} />
          {c.rules.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t('cache.empty')}</p>
          ) : null}
          <ol className="space-y-2">
            {c.rules.map((r, i) => (
              <li
                key={i}
                className="space-y-2 rounded-md border border-border p-3"
                aria-label={t('cache.ruleLabel', { n: i + 1 })}
              >
                <div className="flex flex-wrap items-end gap-2">
                  <NativeSelect
                    id={`cr-kind-${i}`}
                    label={t('forwarders.matchKind')}
                    value={r.match.kind}
                    options={[
                      { value: 'prefix', label: t('forwarders.prefix') },
                      { value: 'exact', label: t('forwarders.exact') },
                      { value: 'regex', label: t('forwarders.regex') },
                    ]}
                    onChange={(v) =>
                      setRule(i, {
                        match: {
                          ...r.match,
                          kind: v as CacheRule['match']['kind'],
                        },
                      })
                    }
                  />
                  <label className="flex min-w-40 flex-1 flex-col gap-1 text-xs">
                    <span className="text-muted-foreground">
                      {t('forwarders.path')}
                    </span>
                    <Input
                      value={r.match.path}
                      onChange={(e) =>
                        setRule(i, {
                          match: { ...r.match, path: e.target.value },
                        })
                      }
                    />
                  </label>
                  <label className="flex w-28 flex-col gap-1 text-xs">
                    <span className="text-muted-foreground">
                      {t('cache.ttl')}
                    </span>
                    <Input
                      type="number"
                      min={0}
                      value={r.ttl_seconds}
                      onChange={(e) =>
                        setRule(i, { ttl_seconds: Number(e.target.value) })
                      }
                    />
                  </label>
                  <label className="flex w-28 flex-col gap-1 text-xs">
                    <span className="text-muted-foreground">
                      {t('cache.swr')}
                    </span>
                    <Input
                      type="number"
                      min={0}
                      value={r.stale_while_revalidate_seconds ?? 0}
                      onChange={(e) =>
                        setRule(i, {
                          stale_while_revalidate_seconds: Number(
                            e.target.value,
                          ),
                        })
                      }
                    />
                  </label>
                  <Button
                    type="button"
                    size="icon-sm"
                    variant="ghost"
                    aria-label={t('common.remove')}
                    onClick={() =>
                      ed.setDraft({
                        ...c,
                        rules: c.rules.filter((_, j) => j !== i),
                      })
                    }
                  >
                    <TrashGlassIcon size="1.2em" />
                  </Button>
                </div>
                <div className="flex flex-wrap items-end gap-2">
                  <label className="flex w-40 flex-col gap-1 text-xs">
                    <span className="text-muted-foreground">
                      {t('cache.statuses')}
                    </span>
                    <Input
                      defaultValue={(r.status_codes ?? [200]).join(', ')}
                      onBlur={(e) =>
                        setRule(i, {
                          status_codes: splitList(e.target.value).map(Number),
                        })
                      }
                    />
                  </label>
                  <label className="flex w-48 flex-col gap-1 text-xs">
                    <span className="text-muted-foreground">
                      {t('cache.vary')}
                    </span>
                    <Input
                      defaultValue={(r.vary ?? []).join(', ')}
                      placeholder="Accept-Language"
                      onBlur={(e) =>
                        setRule(i, { vary: splitList(e.target.value) })
                      }
                    />
                  </label>
                  <CheckboxField
                    id={`cr-override-${i}`}
                    label={t('cache.override')}
                    checked={Boolean(r.override_upstream)}
                    onChange={(v) => setRule(i, { override_upstream: v })}
                  />
                  <CheckboxField
                    id={`cr-cookies-${i}`}
                    label={t('cache.withCookies')}
                    checked={Boolean(r.cache_with_cookies)}
                    onChange={(v) => setRule(i, { cache_with_cookies: v })}
                  />
                </div>
                <div className="flex flex-wrap items-end gap-2">
                  <label className="flex w-40 flex-col gap-1 text-xs">
                    <span className="text-muted-foreground">
                      {t('cache.staleIfError')}
                    </span>
                    <Input
                      type="number"
                      min={0}
                      value={r.stale_if_error_seconds ?? 0}
                      onChange={(e) =>
                        setRule(i, {
                          stale_if_error_seconds: Number(e.target.value),
                        })
                      }
                    />
                  </label>
                  <label className="flex w-48 flex-col gap-1 text-xs">
                    <span className="text-muted-foreground">
                      {t('cache.ignoreQuery')}
                    </span>
                    <Input
                      defaultValue={(r.ignore_query_params ?? []).join(', ')}
                      placeholder="utm_source, fbclid"
                      onBlur={(e) =>
                        setRule(i, {
                          ignore_query_params: splitList(e.target.value),
                        })
                      }
                    />
                  </label>
                  <label className="flex w-48 flex-col gap-1 text-xs">
                    <span className="text-muted-foreground">
                      {t('cache.keyCookies')}
                    </span>
                    <Input
                      defaultValue={(r.key_cookies ?? []).join(', ')}
                      placeholder="lang"
                      onBlur={(e) =>
                        setRule(i, { key_cookies: splitList(e.target.value) })
                      }
                    />
                  </label>
                  <CheckboxField
                    id={`cr-never-${i}`}
                    label={t('cache.never')}
                    checked={Boolean(r.never)}
                    onChange={(v) => setRule(i, { never: v })}
                  />
                </div>
                {Object.entries(ed.errors)
                  .filter(([k]) => k.startsWith(`rules[${i}].`))
                  .map(([k, v]) => (
                    <FieldMessage
                      key={k}
                      message={`${k.replace(`rules[${i}].`, '')}: ${v}`}
                    />
                  ))}
              </li>
            ))}
          </ol>
          <FieldMessage message={ed.errors.rules} />
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={c.rules.length >= policies.limits.max_cache_rules}
            onClick={() =>
              ed.setDraft({
                ...c,
                rules: [
                  ...c.rules,
                  {
                    match: { kind: 'prefix', path: '/assets' },
                    ttl_seconds: 300,
                  },
                ],
              })
            }
          >
            <PlusGlassIcon size="1.2em" />
            {t('cache.add')}
          </Button>
          <p className="text-xs text-muted-foreground">{t('cache.safety')}</p>
        </Section>
        <SaveBar
          saved={saved}
          draft={c}
          dirty={ed.dirty}
          saving={ed.save.isPending}
          error={ed.save.error?.message}
          onSave={ed.onSave}
          onDiscard={ed.discard}
        />
        <CacheStatsCard app={app} domain={domain} />
      </div>
      <PreviewControls
        id="cache-preview"
        method={ed.method}
        path={ed.path}
        onMethod={ed.setMethod}
        onPath={ed.setPath}
        data={ed.preview.data}
        loading={ed.preview.isFetching}
      />
    </div>
  )
}
