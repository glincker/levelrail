import { useTranslation } from 'react-i18next'
import { ShieldCheckIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { HeaderRulesEditor } from './HeaderRulesEditor'
import { PreviewControls } from './PreviewControls'
import {
  CheckboxField,
  FieldMessage,
  NativeSelect,
  SaveBar,
  Section,
} from './PolicyShared'
import { splitList } from './policyUtils'
import { usePolicyEditor } from './usePolicyEditor'
import type {
  CorsPreset,
  DomainPolicies,
  HeadersSpec,
  SecurityPreset,
} from '../../queries/domainPolicyTypes'

const EMPTY: HeadersSpec = { rules: [] }

export function HeadersTab({
  app,
  domain,
  policies,
}: {
  app: string
  domain: string
  policies: DomainPolicies
}) {
  const { t } = useTranslation('domainPolicies')
  const saved = policies.policy.headers ?? EMPTY
  const ed = usePolicyEditor(app, domain, 'headers', saved)
  const d = ed.draft
  const set = (patch: Partial<HeadersSpec>) => ed.setDraft({ ...d, ...patch })
  const setSecurity = (patch: Partial<SecurityPreset>) =>
    d.security && set({ security: { ...d.security, ...patch } })
  const setCors = (patch: Partial<CorsPreset>) =>
    d.cors && set({ cors: { ...d.cors, ...patch } })

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_20rem]">
      <div className="space-y-4">
        <Section
          title={t('headers.presetsTitle')}
          description={t('headers.presetsHelp')}
        >
          <div className="flex flex-wrap gap-2">
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => set({ security: policies.security_preset })}
            >
              <ShieldCheckIcon />
              {t('headers.presetSecurity')}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() =>
                set({
                  cors: d.cors ?? {
                    origins: [],
                    methods: ['GET', 'POST'],
                    preflight: true,
                  },
                })
              }
            >
              {t('headers.presetCors')}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => set({ hide_server: true })}
            >
              {t('headers.presetHideServer')}
            </Button>
          </div>
          <CheckboxField
            id="hide-server"
            label={t('headers.hideServer')}
            checked={Boolean(d.hide_server)}
            onChange={(v) => set({ hide_server: v })}
          />
        </Section>

        {d.security ? (
          <Section title={t('headers.securityTitle')}>
            {!policies.tls_real ? (
              <p className="text-xs text-muted-foreground">
                {t('headers.hstsNeedsTls')}
              </p>
            ) : null}
            <CheckboxField
              id="sec-hsts"
              label={t('headers.hsts')}
              checked={d.security.hsts}
              onChange={(v) => setSecurity({ hsts: v })}
            />
            <FieldMessage message={ed.errors['security.hsts']} />
            <CheckboxField
              id="sec-nosniff"
              label={t('headers.nosniff')}
              checked={d.security.content_type_options}
              onChange={(v) => setSecurity({ content_type_options: v })}
            />
            <NativeSelect
              id="sec-frame"
              label={t('headers.frameOptions')}
              value={d.security.frame_options ?? ''}
              options={[
                { value: '', label: t('headers.off') },
                { value: 'SAMEORIGIN', label: 'SAMEORIGIN' },
                { value: 'DENY', label: 'DENY' },
              ]}
              onChange={(v) => setSecurity({ frame_options: v })}
            />
            <label className="flex flex-col gap-1 text-xs">
              <span className="text-muted-foreground">
                {t('headers.referrerPolicy')}
              </span>
              <Input
                value={d.security.referrer_policy ?? ''}
                onChange={(e) =>
                  setSecurity({ referrer_policy: e.target.value })
                }
              />
            </label>
            <FieldMessage message={ed.errors['security.referrer_policy']} />
            <label className="flex flex-col gap-1 text-xs">
              <span className="text-muted-foreground">
                {t('headers.permissionsPolicy')}
              </span>
              <Input
                value={d.security.permissions_policy ?? ''}
                onChange={(e) =>
                  setSecurity({ permissions_policy: e.target.value })
                }
              />
            </label>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              onClick={() => set({ security: undefined })}
            >
              {t('headers.removeSecurity')}
            </Button>
          </Section>
        ) : null}

        {d.cors ? (
          <Section
            title={t('headers.corsTitle')}
            description={t('headers.corsHelp')}
          >
            <label className="flex flex-col gap-1 text-xs">
              <span className="text-muted-foreground">
                {t('headers.corsOrigins')}
              </span>
              <Input
                defaultValue={d.cors.origins.join(', ')}
                placeholder="https://app.example.com, https://admin.example.com"
                onBlur={(e) => setCors({ origins: splitList(e.target.value) })}
              />
            </label>
            <FieldMessage message={ed.errors['cors.origins']} />
            <label className="flex flex-col gap-1 text-xs">
              <span className="text-muted-foreground">
                {t('headers.corsMethods')}
              </span>
              <Input
                defaultValue={(d.cors.methods ?? []).join(', ')}
                onBlur={(e) =>
                  setCors({
                    methods: splitList(e.target.value).map((m) =>
                      m.toUpperCase(),
                    ),
                  })
                }
              />
            </label>
            <label className="flex flex-col gap-1 text-xs">
              <span className="text-muted-foreground">
                {t('headers.corsHeaders')}
              </span>
              <Input
                defaultValue={(d.cors.headers ?? []).join(', ')}
                onBlur={(e) => setCors({ headers: splitList(e.target.value) })}
              />
            </label>
            <label className="flex flex-col gap-1 text-xs">
              <span className="text-muted-foreground">
                {t('headers.corsMaxAge')}
              </span>
              <Input
                type="number"
                min={0}
                max={86400}
                value={d.cors.max_age_seconds ?? 0}
                onChange={(e) =>
                  setCors({ max_age_seconds: Number(e.target.value) })
                }
              />
            </label>
            <CheckboxField
              id="cors-cred"
              label={t('headers.corsCredentials')}
              checked={Boolean(d.cors.credentials)}
              onChange={(v) => setCors({ credentials: v })}
            />
            <CheckboxField
              id="cors-preflight"
              label={t('headers.corsPreflight')}
              checked={d.cors.preflight}
              onChange={(v) => setCors({ preflight: v })}
            />
            {Object.entries(ed.errors)
              .filter(([k]) => k.startsWith('cors.') && k !== 'cors.origins')
              .map(([k, v]) => (
                <FieldMessage key={k} message={`${k}: ${v}`} />
              ))}
            <Button
              type="button"
              size="sm"
              variant="ghost"
              onClick={() => set({ cors: undefined })}
            >
              {t('headers.removeCors')}
            </Button>
          </Section>
        ) : null}

        <Section
          title={t('headers.rulesTitle')}
          description={t('headers.rulesHelp')}
        >
          <HeaderRulesEditor
            rules={d.rules}
            onChange={(rules) => set({ rules })}
            errors={ed.errors}
            max={policies.limits.max_header_rules}
          />
          <label className="flex flex-col gap-1 text-xs">
            <span className="text-muted-foreground">
              {t('headers.forwardedPrefix')}
            </span>
            <Input
              value={d.forwarded_prefix ?? ''}
              placeholder="/app"
              onChange={(e) => set({ forwarded_prefix: e.target.value })}
            />
          </label>
          <FieldMessage message={ed.errors.forwarded_prefix} />
        </Section>

        <SaveBar
          saved={saved}
          draft={d}
          dirty={ed.dirty}
          saving={ed.save.isPending}
          error={ed.save.error?.message}
          onSave={ed.onSave}
          onDiscard={ed.discard}
        />
      </div>
      <PreviewControls
        id="headers-preview"
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
