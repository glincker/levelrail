import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useVirtualizer } from '@tanstack/react-virtual'
import { ShieldAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { useShieldBots, useUnderAttack } from '../../queries/domainShield'
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
  DomainPolicies,
  ShieldLimits,
  ShieldSpec,
} from '../../queries/domainPolicyTypes'
import { setVisibleInterval } from '../../lib/pageVisibility'

const DEFAULT_SHIELD: ShieldSpec = { enabled: false }
const BOT_ROW_HEIGHT = 28
const CLOCK_TICK_MS = 15_000

function useNow(): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const stop = setVisibleInterval(() => setNow(Date.now()), CLOCK_TICK_MS)
    return () => stop()
  }, [])
  return now
}

function attackOptions(limits: ShieldLimits): number[] {
  const all = [15, 60, limits.attack_default_minutes, limits.attack_max_minutes]
  return [...new Set(all)]
    .filter((m) => m > 0 && m <= limits.attack_max_minutes)
    .sort((a, b) => a - b)
}

function UnderAttackPanel({
  app,
  domain,
  until,
  limits,
}: {
  app: string
  domain: string
  until?: string
  limits: ShieldLimits
}) {
  const { t } = useTranslation('domainPolicies')
  const { start, stop } = useUnderAttack(app, domain)
  const now = useNow()
  const options = useMemo(() => attackOptions(limits), [limits])
  const [minutes, setMinutes] = useState(limits.attack_default_minutes)
  const untilMs = until ? Date.parse(until) : 0
  const active = untilMs > now
  const error = start.error?.message ?? stop.error?.message

  return (
    <Section
      title={t('shield.attack.title')}
      description={t('shield.attack.help', {
        rate: limits.attack_rate_per_minute,
        burst: limits.attack_burst,
        concurrent: limits.attack_max_concurrent,
      })}
    >
      {active ? (
        <Alert variant="destructive">
          <ShieldAlertGlassIcon
            className="text-tone-warning"
            size="1.4em"
            aria-hidden="true"
          />
          <AlertDescription>
            {t('shield.attack.active', {
              time: new Date(untilMs).toLocaleTimeString(),
            })}
          </AlertDescription>
        </Alert>
      ) : (
        <p className="text-xs text-muted-foreground">
          {t('shield.attack.idle')}
        </p>
      )}
      <div className="flex flex-wrap items-end gap-2">
        {active ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={stop.isPending}
            onClick={() => stop.mutate()}
          >
            {t('shield.attack.standDown')}
          </Button>
        ) : (
          <>
            <NativeSelect
              id="shield-attack-minutes"
              label={t('shield.attack.duration')}
              value={String(minutes)}
              options={options.map((m) => ({
                value: String(m),
                label: t('shield.attack.minutes', { count: m }),
              }))}
              onChange={(v) => setMinutes(Number(v))}
            />
            <Button
              type="button"
              size="sm"
              variant="destructive"
              disabled={start.isPending}
              onClick={() => start.mutate(minutes)}
            >
              {t('shield.attack.start')}
            </Button>
          </>
        )}
      </div>
      <FieldMessage message={error} />
    </Section>
  )
}

function BotList() {
  const { t } = useTranslation('domainPolicies')
  const { data, error, isLoading, refetch } = useShieldBots()
  const parentRef = useRef<HTMLDivElement>(null)
  const agents = data?.agents ?? []
  const virtualizer = useVirtualizer({
    count: agents.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => BOT_ROW_HEIGHT,
    overscan: 8,
  })
  if (isLoading) {
    return (
      <p className="text-xs text-muted-foreground">
        {t('shield.bots.loading')}
      </p>
    )
  }
  if (error || !data) {
    return (
      <EmptyState
        icon={<ShieldAlertGlassIcon className="size-6 text-tone-warning" />}
        title={t('shield.bots.failed')}
        description={error?.message ?? ''}
        action={
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => void refetch()}
          >
            {t('page.retry')}
          </Button>
        }
      />
    )
  }
  if (agents.length === 0) {
    return (
      <EmptyState
        icon={<ShieldAlertGlassIcon className="size-6 text-tone-warning" />}
        title={t('shield.bots.emptyTitle')}
        description={t('shield.bots.emptyHelp')}
      />
    )
  }
  return (
    <div className="space-y-1">
      <p className="text-xs text-muted-foreground">
        {data.custom
          ? t('shield.bots.custom', { count: agents.length })
          : t('shield.bots.shipped', { count: agents.length })}
      </p>
      <div
        ref={parentRef}
        className="max-h-48 overflow-auto rounded-md border border-border"
        role="list"
        aria-label={t('shield.bots.title')}
      >
        <div
          className="relative"
          style={{ height: virtualizer.getTotalSize() }}
        >
          {virtualizer.getVirtualItems().map((row) => (
            <div
              key={row.key}
              role="listitem"
              className="absolute top-0 left-0 w-full px-2 py-1 font-mono text-xs"
              style={{ transform: `translateY(${row.start}px)` }}
            >
              {agents[row.index]}
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

function NumberField({
  id,
  label,
  value,
  hint,
  error,
  onChange,
}: {
  id: string
  label: string
  value?: number
  hint?: string
  error?: string
  onChange: (v: number | undefined) => void
}) {
  return (
    <label htmlFor={id} className="flex w-44 flex-col gap-1 text-xs">
      <span className="text-muted-foreground">{label}</span>
      <Input
        id={id}
        type="number"
        min={0}
        value={value ?? ''}
        placeholder={hint}
        onChange={(e) =>
          onChange(e.target.value === '' ? undefined : Number(e.target.value))
        }
      />
      <FieldMessage message={error} />
    </label>
  )
}

export function ShieldTab({
  app,
  domain,
  policies,
}: {
  app: string
  domain: string
  policies: DomainPolicies
}) {
  const { t } = useTranslation('domainPolicies')
  const stored = policies.policy.shield
  const saved = useMemo<ShieldSpec>(
    () => ({ ...(stored ?? DEFAULT_SHIELD), under_attack_until: undefined }),
    [stored],
  )
  const ed = usePolicyEditor(app, domain, 'shield', saved)
  const g = ed.draft
  const limits = policies.limits.shield
  const set = (patch: Partial<ShieldSpec>) => ed.setDraft({ ...g, ...patch })
  const restFields = Object.entries(ed.errors).filter(
    ([k]) =>
      ![
        'rate_per_minute',
        'burst',
        'max_concurrent',
        'max_body_bytes',
        'retry_after_seconds',
        'tarpit_ms',
      ].includes(k),
  )

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_20rem]">
      <div className="space-y-4">
        <UnderAttackPanel
          app={app}
          domain={domain}
          until={stored?.under_attack_until}
          limits={limits}
        />
        <Section title={t('shield.title')} description={t('shield.help')}>
          <div className="flex flex-wrap items-center gap-2">
            <CheckboxField
              id="shield-enabled"
              label={t('shield.enabled')}
              checked={g.enabled}
              onChange={(v) => set({ enabled: v })}
            />
            <Badge variant={g.enabled ? 'success' : 'muted'}>
              {g.enabled ? t('shield.on') : t('shield.off')}
            </Badge>
          </div>
          <div className="flex flex-wrap gap-3">
            <NumberField
              id="shield-rate"
              label={t('shield.rate')}
              value={g.rate_per_minute}
              hint={t('shield.off')}
              error={ed.errors.rate_per_minute}
              onChange={(v) => set({ rate_per_minute: v })}
            />
            <NumberField
              id="shield-burst"
              label={t('shield.burst')}
              value={g.burst}
              hint={t('shield.burstDefault')}
              error={ed.errors.burst}
              onChange={(v) => set({ burst: v })}
            />
            <NumberField
              id="shield-concurrent"
              label={t('shield.concurrent')}
              value={g.max_concurrent}
              hint={t('shield.off')}
              error={ed.errors.max_concurrent}
              onChange={(v) => set({ max_concurrent: v })}
            />
            <NumberField
              id="shield-body"
              label={t('shield.body')}
              value={g.max_body_bytes}
              hint={t('shield.off')}
              error={ed.errors.max_body_bytes}
              onChange={(v) => set({ max_body_bytes: v })}
            />
          </div>
          <p className="text-xs text-muted-foreground">
            {t('shield.perClient', { count: limits.max_tracked_ips })}
          </p>
          <div className="flex flex-wrap gap-3">
            <NumberField
              id="shield-retry"
              label={t('shield.retryAfter')}
              value={g.retry_after_seconds}
              hint={String(limits.default_retry_after_seconds)}
              error={ed.errors.retry_after_seconds}
              onChange={(v) => set({ retry_after_seconds: v })}
            />
            <NumberField
              id="shield-tarpit"
              label={t('shield.tarpit', { max: limits.tarpit_max_ms })}
              value={g.tarpit_ms}
              hint={t('shield.off')}
              error={ed.errors.tarpit_ms}
              onChange={(v) => set({ tarpit_ms: v })}
            />
          </div>
          <CheckboxField
            id="shield-bots"
            label={t('shield.blockBots')}
            checked={g.block_bots ?? false}
            onChange={(v) => set({ block_bots: v })}
          />
          <label className="flex flex-col gap-1 text-xs">
            <span className="text-muted-foreground">
              {t('shield.botAgents', { max: limits.max_bot_agents })}
            </span>
            <Textarea
              defaultValue={(g.bot_agents ?? []).join('\n')}
              rows={3}
              placeholder="examplebot"
              onBlur={(e) => set({ bot_agents: splitList(e.target.value) })}
            />
          </label>
          <label className="flex flex-col gap-1 text-xs">
            <span className="text-muted-foreground">{t('shield.allow')}</span>
            <Textarea
              defaultValue={(g.allow ?? []).join('\n')}
              rows={3}
              placeholder="203.0.113.0/24"
              onBlur={(e) => set({ allow: splitList(e.target.value) })}
            />
          </label>
          {restFields.map(([k, v]) => (
            <FieldMessage key={k} message={k ? `${k}: ${v}` : v} />
          ))}
          <p className="text-xs text-muted-foreground">{t('shield.privacy')}</p>
        </Section>
        <SaveBar
          saved={saved}
          draft={g}
          dirty={ed.dirty}
          saving={ed.save.isPending}
          error={ed.save.error?.message}
          onSave={() =>
            ed.save.mutate({
              ...g,
              under_attack_until: stored?.under_attack_until,
            })
          }
          onDiscard={ed.discard}
        />
        <Section
          title={t('shield.bots.title')}
          description={t('shield.bots.help')}
        >
          <BotList />
        </Section>
      </div>
      <PreviewControls
        id="shield-preview"
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
