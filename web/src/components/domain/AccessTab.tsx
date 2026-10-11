import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { GlobeHemisphereWestIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { DomainBasicAuthControl } from '../DomainBasicAuthControl'
import { DomainWafControl } from '../DomainWafControl'
import { useGeoLookup } from '../../queries/domainPolicies'
import { PreviewControls } from './PreviewControls'
import { FieldMessage, NativeSelect, SaveBar, Section } from './PolicyShared'
import { splitList } from './policyUtils'
import { usePolicyEditor } from './usePolicyEditor'
import type { DomainPolicies, GeoSpec } from '../../queries/domainPolicyTypes'

const DEFAULT_GEO: GeoSpec = {
  mode: 'deny',
  countries: [],
  action: 'block',
  unknown: 'allow',
}

function GeoLookupTool() {
  const { t } = useTranslation('domainPolicies')
  const [input, setInput] = useState('')
  const [ip, setIp] = useState('')
  const { data, error, isFetching } = useGeoLookup(ip)
  return (
    <form
      className="space-y-2"
      onSubmit={(e) => {
        e.preventDefault()
        setIp(input.trim())
      }}
    >
      <label className="flex flex-col gap-1 text-xs">
        <span className="text-muted-foreground">{t('geo.lookupLabel')}</span>
        <div className="flex gap-2">
          <Input
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder="203.0.113.7"
          />
          <Button
            type="submit"
            size="sm"
            variant="outline"
            disabled={isFetching}
          >
            {t('geo.lookup')}
          </Button>
        </div>
      </label>
      {error ? <FieldMessage message={error.message} /> : null}
      {data?.ip ? (
        <p className="text-sm" aria-live="polite">
          {data.country
            ? t('geo.lookupResult', {
                ip: data.ip,
                country: data.country,
                source: data.source,
              })
            : data.note}
        </p>
      ) : null}
    </form>
  )
}

export function AccessTab({
  app,
  domain,
  policies,
}: {
  app: string
  domain: string
  policies: DomainPolicies
}) {
  const { t } = useTranslation('domainPolicies')
  const saved = policies.policy.geo ?? DEFAULT_GEO
  const ed = usePolicyEditor(app, domain, 'geo', saved)
  const g = ed.draft
  const set = (patch: Partial<GeoSpec>) => ed.setDraft({ ...g, ...patch })
  const geo = policies.geo

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_20rem]">
      <div className="space-y-4">
        <Section title={t('geo.title')} description={t('geo.help')}>
          <div className="flex flex-wrap items-center gap-2 text-xs">
            <GlobeHemisphereWestIcon className="size-4" aria-hidden="true" />
            {geo.active ? (
              <Badge variant="success">
                {t('geo.sourceActive', { sources: geo.sources.join(', ') })}
              </Badge>
            ) : (
              <Badge variant="warning">{t('geo.sourceNone')}</Badge>
            )}
            {geo.header_needs ? (
              <span className="text-muted-foreground">{geo.header_needs}</span>
            ) : null}
            {geo.error ? (
              <span className="text-destructive">{geo.error}</span>
            ) : null}
          </div>
          {!geo.active ? (
            <p className="text-xs text-muted-foreground">
              {t('geo.sourceHelp')}
            </p>
          ) : null}
          <div className="flex flex-wrap gap-2">
            <NativeSelect
              id="geo-mode"
              label={t('geo.mode')}
              value={g.mode}
              options={[
                { value: 'deny', label: t('geo.modeDeny') },
                { value: 'allow', label: t('geo.modeAllow') },
              ]}
              onChange={(v) => set({ mode: v as GeoSpec['mode'] })}
            />
            <NativeSelect
              id="geo-action"
              label={t('geo.action')}
              value={g.action}
              options={[
                { value: 'block', label: t('geo.actionBlock') },
                { value: 'redirect', label: t('geo.actionRedirect') },
                { value: 'error_page', label: t('geo.actionPage') },
              ]}
              onChange={(v) => set({ action: v as GeoSpec['action'] })}
            />
            <NativeSelect
              id="geo-unknown"
              label={t('geo.unknown')}
              value={g.unknown ?? 'allow'}
              options={[
                { value: 'allow', label: t('geo.unknownAllow') },
                { value: 'block', label: t('geo.unknownBlock') },
              ]}
              onChange={(v) => set({ unknown: v as GeoSpec['unknown'] })}
            />
            {g.action !== 'redirect' ? (
              <NativeSelect
                id="geo-status"
                label={t('geo.status')}
                value={String(g.status_code ?? 403)}
                options={['403', '404', '451'].map((s) => ({
                  value: s,
                  label: s,
                }))}
                onChange={(v) => set({ status_code: Number(v) })}
              />
            ) : null}
          </div>
          <label className="flex flex-col gap-1 text-xs">
            <span className="text-muted-foreground">{t('geo.countries')}</span>
            <Input
              defaultValue={g.countries.join(', ')}
              placeholder="US, CA, GB"
              onBlur={(e) =>
                set({
                  countries: splitList(e.target.value).map((c) =>
                    c.toUpperCase(),
                  ),
                })
              }
            />
          </label>
          {Object.entries(ed.errors)
            .filter(([k]) => k.startsWith('countries'))
            .map(([k, v]) => (
              <FieldMessage key={k} message={v} />
            ))}
          {g.action === 'redirect' ? (
            <label className="flex flex-col gap-1 text-xs">
              <span className="text-muted-foreground">
                {t('geo.redirectUrl')}
              </span>
              <Input
                value={g.redirect_url ?? ''}
                onChange={(e) => set({ redirect_url: e.target.value })}
              />
              <FieldMessage message={ed.errors.redirect_url} />
            </label>
          ) : null}
          {g.action === 'error_page' ? (
            <label className="flex flex-col gap-1 text-xs">
              <span className="text-muted-foreground">{t('geo.body')}</span>
              <Textarea
                value={g.body ?? ''}
                rows={4}
                onChange={(e) => set({ body: e.target.value })}
              />
              <FieldMessage message={ed.errors.body} />
            </label>
          ) : null}
          <label className="flex flex-col gap-1 text-xs">
            <span className="text-muted-foreground">{t('geo.exempt')}</span>
            <Textarea
              defaultValue={(g.exempt ?? []).join('\n')}
              rows={3}
              placeholder="203.0.113.0/24"
              onBlur={(e) => set({ exempt: splitList(e.target.value) })}
            />
          </label>
          {Object.entries(ed.errors)
            .filter(
              ([k]) => k.startsWith('exempt') || k === 'mode' || k === 'action',
            )
            .map(([k, v]) => (
              <FieldMessage key={k} message={`${k}: ${v}`} />
            ))}
          <p className="text-xs text-muted-foreground">
            {t('geo.privateNote')}
          </p>
        </Section>
        <SaveBar
          saved={saved}
          draft={g}
          dirty={ed.dirty}
          saving={ed.save.isPending}
          error={ed.save.error?.message}
          onSave={ed.onSave}
          onDiscard={ed.discard}
        />
        <Section title={t('geo.lookupTitle')}>
          <GeoLookupTool />
        </Section>
        <Section
          title={t('access.basicAuth')}
          description={t('access.basicAuthHelp')}
        >
          <DomainBasicAuthControl appName={app} domain={domain} />
        </Section>
        <Section title={t('access.waf')} description={t('access.wafHelp')}>
          <DomainWafControl appName={app} domain={domain} />
        </Section>
      </div>
      <PreviewControls
        id="geo-preview"
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
