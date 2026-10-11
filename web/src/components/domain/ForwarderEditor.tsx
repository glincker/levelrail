import { useTranslation } from 'react-i18next'
import {
  ArrowDownIcon,
  ArrowUpIcon,
  PlusIcon,
  TrashIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { CheckboxField, FieldMessage, NativeSelect } from './PolicyShared'
import { moveItem, splitList } from './policyUtils'
import type { Forwarder } from '../../queries/domainPolicyTypes'

function errorsUnder(errors: Record<string, string>, prefix: string) {
  return Object.entries(errors).filter(([k]) => k.startsWith(prefix))
}

export function ForwarderEditor({
  rules,
  onChange,
  errors,
  apps,
  max,
}: {
  rules: Forwarder[]
  onChange: (rules: Forwarder[]) => void
  errors: Record<string, string>
  apps: string[]
  max: number
}) {
  const { t } = useTranslation('domainPolicies')
  const update = (i: number, patch: Partial<Forwarder>) =>
    onChange(rules.map((r, j) => (j === i ? { ...r, ...patch } : r)))

  return (
    <div className="space-y-2">
      {rules.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('forwarders.empty')}</p>
      ) : null}
      <ol className="space-y-2">
        {rules.map((r, i) => (
          <li
            key={i}
            className="space-y-2 rounded-md border border-border p-3"
            aria-label={t('forwarders.ruleLabel', { n: i + 1 })}
          >
            <div className="flex flex-wrap items-end gap-2">
              <NativeSelect
                id={`fw-kind-${i}`}
                label={t('forwarders.matchKind')}
                value={r.match.kind}
                options={[
                  { value: 'prefix', label: t('forwarders.prefix') },
                  { value: 'exact', label: t('forwarders.exact') },
                  { value: 'regex', label: t('forwarders.regex') },
                ]}
                onChange={(v) =>
                  update(i, {
                    match: {
                      ...r.match,
                      kind: v as Forwarder['match']['kind'],
                    },
                    strip_prefix: v === 'prefix' ? r.strip_prefix : false,
                  })
                }
              />
              <label className="flex min-w-40 flex-1 flex-col gap-1 text-xs">
                <span className="text-muted-foreground">
                  {t('forwarders.path')}
                </span>
                <Input
                  value={r.match.path}
                  placeholder="/api"
                  aria-invalid={Boolean(errors[`rules[${i}].match.path`])}
                  onChange={(e) =>
                    update(i, { match: { ...r.match, path: e.target.value } })
                  }
                />
              </label>
              <label className="flex w-40 flex-col gap-1 text-xs">
                <span className="text-muted-foreground">
                  {t('forwarders.methods')}
                </span>
                <Input
                  defaultValue={(r.match.methods ?? []).join(', ')}
                  placeholder={t('forwarders.anyMethod')}
                  onBlur={(e) => {
                    const methods = splitList(e.target.value).map((m) =>
                      m.toUpperCase(),
                    )
                    update(i, {
                      match: {
                        ...r.match,
                        methods: methods.length ? methods : undefined,
                      },
                    })
                  }}
                />
              </label>
              <NativeSelect
                id={`fw-action-${i}`}
                label={t('forwarders.action')}
                value={r.action}
                options={[
                  { value: 'app', label: t('forwarders.actionApp') },
                  { value: 'url', label: t('forwarders.actionUrl') },
                  { value: 'redirect', label: t('forwarders.actionRedirect') },
                ]}
                onChange={(v) =>
                  update(i, { action: v as Forwarder['action'] })
                }
              />
              <div className="flex gap-1">
                <Button
                  type="button"
                  size="icon-sm"
                  variant="ghost"
                  aria-label={t('common.moveUp')}
                  disabled={i === 0}
                  onClick={() => onChange(moveItem(rules, i, i - 1))}
                >
                  <ArrowUpIcon />
                </Button>
                <Button
                  type="button"
                  size="icon-sm"
                  variant="ghost"
                  aria-label={t('common.moveDown')}
                  disabled={i === rules.length - 1}
                  onClick={() => onChange(moveItem(rules, i, i + 1))}
                >
                  <ArrowDownIcon />
                </Button>
                <Button
                  type="button"
                  size="icon-sm"
                  variant="ghost"
                  aria-label={t('common.remove')}
                  onClick={() => onChange(rules.filter((_, j) => j !== i))}
                >
                  <TrashIcon />
                </Button>
              </div>
            </div>
            <div className="flex flex-wrap items-end gap-2">
              {r.action === 'app' ? (
                <NativeSelect
                  id={`fw-app-${i}`}
                  label={t('forwarders.targetApp')}
                  value={r.app ?? ''}
                  options={[
                    { value: '', label: t('forwarders.chooseApp') },
                    ...apps.map((a) => ({ value: a, label: a })),
                  ]}
                  onChange={(v) => update(i, { app: v })}
                />
              ) : (
                <label className="flex min-w-60 flex-1 flex-col gap-1 text-xs">
                  <span className="text-muted-foreground">
                    {r.action === 'url'
                      ? t('forwarders.targetUrl')
                      : t('forwarders.redirectTo')}
                  </span>
                  <Input
                    value={r.url ?? ''}
                    placeholder="https://api.example.com"
                    aria-invalid={Boolean(errors[`rules[${i}].url`])}
                    onChange={(e) => update(i, { url: e.target.value })}
                  />
                </label>
              )}
              {r.action === 'redirect' ? (
                <NativeSelect
                  id={`fw-status-${i}`}
                  label={t('forwarders.status')}
                  value={String(r.redirect_status ?? 302)}
                  options={['301', '302', '307', '308'].map((s) => ({
                    value: s,
                    label: s,
                  }))}
                  onChange={(v) => update(i, { redirect_status: Number(v) })}
                />
              ) : (
                <>
                  <label className="flex w-36 flex-col gap-1 text-xs">
                    <span className="text-muted-foreground">
                      {t('forwarders.rewritePrefix')}
                    </span>
                    <Input
                      value={r.rewrite_prefix ?? ''}
                      placeholder="/v2"
                      onChange={(e) =>
                        update(i, { rewrite_prefix: e.target.value })
                      }
                    />
                  </label>
                  <NativeSelect
                    id={`fw-host-${i}`}
                    label={t('forwarders.host')}
                    value={r.host ?? ''}
                    options={[
                      { value: '', label: t('forwarders.hostDefault') },
                      {
                        value: 'preserve',
                        label: t('forwarders.hostPreserve'),
                      },
                      {
                        value: 'upstream',
                        label: t('forwarders.hostUpstream'),
                      },
                      { value: 'custom', label: t('forwarders.hostCustom') },
                    ]}
                    onChange={(v) =>
                      update(i, { host: v as Forwarder['host'] })
                    }
                  />
                  {r.host === 'custom' ? (
                    <label className="flex w-48 flex-col gap-1 text-xs">
                      <span className="text-muted-foreground">
                        {t('forwarders.hostValue')}
                      </span>
                      <Input
                        value={r.host_value ?? ''}
                        onChange={(e) =>
                          update(i, { host_value: e.target.value })
                        }
                      />
                    </label>
                  ) : null}
                  <label className="flex w-28 flex-col gap-1 text-xs">
                    <span className="text-muted-foreground">
                      {t('forwarders.timeout')}
                    </span>
                    <Input
                      type="number"
                      min={0}
                      value={r.timeout_seconds ?? 0}
                      onChange={(e) =>
                        update(i, { timeout_seconds: Number(e.target.value) })
                      }
                    />
                  </label>
                </>
              )}
            </div>
            {r.action !== 'redirect' ? (
              <div className="flex flex-wrap gap-4">
                {r.match.kind === 'prefix' ? (
                  <CheckboxField
                    id={`fw-strip-${i}`}
                    label={t('forwarders.stripPrefix')}
                    checked={Boolean(r.strip_prefix)}
                    onChange={(v) => update(i, { strip_prefix: v })}
                  />
                ) : null}
                <CheckboxField
                  id={`fw-ws-${i}`}
                  label={t('forwarders.websocket')}
                  checked={r.websocket !== false}
                  onChange={(v) => update(i, { websocket: v })}
                />
              </div>
            ) : null}
            {errorsUnder(errors, `rules[${i}].`).map(([k, v]) => (
              <FieldMessage
                key={k}
                message={`${k.replace(`rules[${i}].`, '')}: ${v}`}
              />
            ))}
          </li>
        ))}
      </ol>
      <FieldMessage message={errors.rules} />
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={rules.length >= max}
        onClick={() =>
          onChange([
            ...rules,
            { match: { kind: 'prefix', path: '/' }, action: 'app', app: '' },
          ])
        }
      >
        <PlusIcon />
        {t('forwarders.add')}
      </Button>
    </div>
  )
}
