import { useEffect, useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { LightbulbGlassIcon, ShieldCheckGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import { useAbuseDryRun, useAppAbuse, useSaveAbuse } from '../../queries/abuse'
import type {
  AbuseCounters,
  AbuseMode,
  AbusePolicy,
  AppAbuse,
} from '../../types/abuse'
import {
  draftFrom,
  draftToPolicy,
  hasProtection,
  policyKey,
  type AbuseDraft,
} from './abuseDraft'

const MODES: AbuseMode[] = ['off', 'log_only', 'enforce']
const PREVIEW_DEBOUNCE_MS = 500

function conditionVariant(a: AppAbuse): 'success' | 'warning' | 'muted' {
  if (a.condition.status === 'True') return 'success'
  return a.condition.reason === 'Off' ? 'muted' : 'warning'
}

function useDebounced<T>(value: T, ms: number): T {
  const [v, setV] = useState(value)
  useEffect(() => {
    const id = setTimeout(() => {
      setV(value)
    }, ms)
    return () => {
      clearTimeout(id)
    }
  }, [value, ms])
  return v
}

function CounterLine({ counters }: Readonly<{ counters: AbuseCounters }>) {
  const { t } = useTranslation('trafficDetail')
  return (
    <p className="text-xs text-muted-foreground tabular-nums">
      {t('abuse.counters', {
        allowed: counters.allowed,
        wouldBlock: counters.would_block,
        blocked: counters.blocked,
      })}
      {counters.fail_open > 0
        ? ` ${t('abuse.failOpen', { count: counters.fail_open })}`
        : ''}
    </p>
  )
}

function ModeField({
  mode,
  previewReady,
  onChange,
}: Readonly<{
  mode: AbuseMode
  previewReady: boolean
  onChange: (m: AbuseMode) => void
}>) {
  const { t } = useTranslation('trafficDetail')
  return (
    <div className="space-y-2">
      <p className="text-sm font-medium">{t('abuse.modeLabel')}</p>
      <div
        role="group"
        aria-label={t('abuse.modeLabel')}
        className="flex flex-wrap gap-1"
      >
        {MODES.map((m) => (
          <Button
            key={m}
            type="button"
            size="sm"
            variant={m === mode ? 'secondary' : 'outline'}
            aria-pressed={m === mode}
            disabled={m === 'enforce' && m !== mode && !previewReady}
            onClick={() => {
              onChange(m)
            }}
          >
            {t(`abuse.modes.${m}`)}
          </Button>
        ))}
      </div>
      <p className="text-xs text-muted-foreground">
        {t(`abuse.modeHints.${mode}`)}
      </p>
    </div>
  )
}

function RateFields({
  draft,
  suggestion,
  onChange,
}: Readonly<{
  draft: AbuseDraft
  suggestion: AppAbuse['suggestion']
  onChange: (patch: Partial<AbuseDraft>) => void
}>) {
  const { t } = useTranslation('trafficDetail')
  const rpsId = useId()
  const burstId = useId()
  return (
    <div className="space-y-2">
      <p className="text-sm font-medium">{t('abuse.rateLabel')}</p>
      <div className="grid grid-cols-2 gap-2">
        <div className="space-y-1">
          <label htmlFor={rpsId} className="text-xs text-muted-foreground">
            {t('abuse.rps')}
          </label>
          <Input
            id={rpsId}
            inputMode="decimal"
            value={draft.rps}
            onChange={(e) => {
              onChange({ rps: e.target.value })
            }}
          />
        </div>
        <div className="space-y-1">
          <label htmlFor={burstId} className="text-xs text-muted-foreground">
            {t('abuse.burst')}
          </label>
          <Input
            id={burstId}
            inputMode="numeric"
            value={draft.burst}
            onChange={(e) => {
              onChange({ burst: e.target.value })
            }}
          />
        </div>
      </div>
      <p className="flex flex-wrap items-center gap-x-2 text-xs text-muted-foreground">
        <LightbulbGlassIcon className="size-5 shrink-0" aria-hidden="true" />
        <span>
          {suggestion.from_traffic
            ? t('abuse.suggestionFromPeak', {
                peak: Math.round(suggestion.peak_rps * 10) / 10,
                rps: suggestion.rps,
                burst: suggestion.burst,
              })
            : t('abuse.suggestionDefault', {
                rps: suggestion.rps,
                burst: suggestion.burst,
              })}
        </span>
        <Button
          type="button"
          variant="link"
          size="xs"
          onClick={() => {
            onChange({
              rps: String(suggestion.rps),
              burst: String(suggestion.burst),
            })
          }}
        >
          {t('abuse.useSuggestion')}
        </Button>
      </p>
    </div>
  )
}

function DenyFields({
  draft,
  onChange,
}: Readonly<{
  draft: AbuseDraft
  onChange: (patch: Partial<AbuseDraft>) => void
}>) {
  const { t } = useTranslation('trafficDetail')
  const cidrId = useId()
  const agentId = useId()
  const uaId = useId()
  return (
    <div className="space-y-2">
      <div className="grid grid-cols-2 gap-2">
        <div className="space-y-1">
          <label htmlFor={cidrId} className="text-xs text-muted-foreground">
            {t('abuse.denyCidrs')}
          </label>
          <Textarea
            id={cidrId}
            rows={2}
            className="min-h-0 font-mono text-xs"
            placeholder={t('abuse.denyCidrsPlaceholder')}
            value={draft.cidrs}
            onChange={(e) => {
              onChange({ cidrs: e.target.value })
            }}
          />
        </div>
        <div className="space-y-1">
          <label htmlFor={agentId} className="text-xs text-muted-foreground">
            {t('abuse.denyAgents')}
          </label>
          <Textarea
            id={agentId}
            rows={2}
            className="min-h-0 font-mono text-xs"
            placeholder={t('abuse.denyAgentsPlaceholder')}
            value={draft.agents}
            onChange={(e) => {
              onChange({ agents: e.target.value })
            }}
          />
        </div>
      </div>
      <label htmlFor={uaId} className="flex items-center gap-2 text-sm">
        <Checkbox
          id={uaId}
          checked={draft.requireUa}
          onCheckedChange={(checked) => {
            onChange({ requireUa: checked === true })
          }}
        />
        {t('abuse.requireUa')}
      </label>
    </div>
  )
}

function Preview({
  app,
  policy,
}: Readonly<{ app: string; policy: AbusePolicy | null }>) {
  const { t } = useTranslation('trafficDetail')
  const dry = useAbuseDryRun(app, policy)
  if (!policy) return <p className="text-sm">{t('abuse.previewNeedsRule')}</p>
  if (dry.isError) {
    return (
      <p className="text-sm text-destructive" role="alert">
        {dry.error.message}
      </p>
    )
  }
  if (!dry.data) {
    return <Skeleton className="h-5 w-64" />
  }
  const r = dry.data
  if (r.requests === 0) {
    return <p className="text-sm">{t('abuse.previewEmpty')}</p>
  }
  const reasons = r.by_reason.filter((b) => b.count > 0)
  return (
    <div className="space-y-1 text-sm" role="status">
      <p>
        <span className="font-medium tabular-nums">
          {t('abuse.previewResult', {
            blocked: r.would_block,
            total: r.requests,
            percent: r.percent.toFixed(1),
          })}
        </span>{' '}
        <span className="text-muted-foreground">
          {t('abuse.previewWindow', {
            minutes: Math.max(1, Math.round(r.covered_minutes)),
          })}
        </span>
      </p>
      {reasons.length > 0 ? (
        <p className="text-xs text-muted-foreground">
          {reasons
            .map((b) => `${t(`abuse.reasons.${b.reason}`)}: ${b.count}`)
            .join(', ')}
        </p>
      ) : null}
      {r.top_clients.length > 0 ? (
        <p className="text-xs text-muted-foreground">
          {t('abuse.topClients', {
            clients: r.top_clients
              .slice(0, 3)
              .map((c) => `${c.client} (${c.would_block})`)
              .join(', '),
          })}
        </p>
      ) : null}
    </div>
  )
}

function CardBody({
  app,
  data,
  counters,
}: Readonly<{ app: string; data: AppAbuse; counters: AbuseCounters }>) {
  const { t } = useTranslation('trafficDetail')
  const [draft, setDraft] = useState<AbuseDraft>(() => draftFrom(data))
  const save = useSaveAbuse(app)
  const policy = draftToPolicy(draft)
  const previewPolicy = useMemo<AbusePolicy | null>(
    () =>
      policy && hasProtection(policy) ? { ...policy, mode: 'enforce' } : null,
    [policy],
  )
  const previewKey = previewPolicy ? policyKey(previewPolicy) : ''
  const settled = useDebounced(previewKey, PREVIEW_DEBOUNCE_MS)
  const settledPolicy = settled === previewKey ? previewPolicy : null
  const dry = useAbuseDryRun(app, settledPolicy)
  const previewReady = !!settledPolicy && !!dry.data && !dry.isFetching
  const saved = data.configured ? data.policy : null
  const dirty = !saved || !policy || policyKey(saved) !== policyKey(policy)
  const startEnforcing =
    !!saved && saved.mode === 'log_only' && !dirty && draft.mode === 'log_only'
  const needsPreview = startEnforcing || draft.mode === 'enforce'
  const canSave =
    !!policy &&
    !save.isPending &&
    (startEnforcing || dirty) &&
    (draft.mode === 'off' || hasProtection(policy)) &&
    (!needsPreview || previewReady)

  function patch(p: Partial<AbuseDraft>) {
    setDraft((d) => ({ ...d, ...p }))
  }

  function submit() {
    if (!policy) return
    const body: AbusePolicy = startEnforcing
      ? { ...policy, mode: 'enforce' }
      : policy
    save.mutate(body, {
      onSuccess: (res) => {
        setDraft(draftFrom(res))
        toast.add({
          title: t('abuse.saved', { mode: t(`abuse.modes.${body.mode}`) }),
          type: 'success',
        })
      },
      onError: (e) => {
        toast.add({ title: e.message, type: 'error' })
      },
    })
  }

  const label = startEnforcing
    ? t('abuse.startEnforcing')
    : t(draft.mode === 'log_only' ? 'abuse.saveLogOnly' : 'abuse.save')

  return (
    <>
      <div className="grid gap-4 px-4 pb-4 lg:grid-cols-3">
        <ModeField
          mode={draft.mode}
          previewReady={previewReady}
          onChange={(mode) => {
            patch({ mode })
          }}
        />
        <RateFields
          draft={draft}
          suggestion={data.suggestion}
          onChange={patch}
        />
        <DenyFields draft={draft} onChange={patch} />
      </div>
      {!policy ? (
        <p className="px-4 pb-3 text-xs text-destructive" role="alert">
          {t('abuse.invalidNumber')}
        </p>
      ) : null}
      <div className="flex flex-wrap items-center justify-between gap-3 border-t border-foreground/10 px-4 py-3">
        <div className="min-w-0 flex-1 space-y-1">
          <Preview app={app} policy={settledPolicy} />
          {!previewReady && draft.mode !== 'off' ? (
            <p className="text-xs text-muted-foreground">
              {t('abuse.enforceNeedsPreview')}
            </p>
          ) : null}
          <CounterLine counters={counters} />
        </div>
        <Button type="button" size="sm" disabled={!canSave} onClick={submit}>
          {label}
        </Button>
      </div>
    </>
  )
}

/**
 * AbuseDefenseCard is the per-app ingress abuse defense on the Traffic tab:
 * mode, per-client rate, deny lists and a live preview of what the draft would
 * have blocked. Enforcing is only offered once that preview has loaded.
 */
export function AbuseDefenseCard({
  app,
  counters,
}: Readonly<{ app: string; counters: AbuseCounters }>) {
  const { t } = useTranslation('trafficDetail')
  const { data, error, isPending } = useAppAbuse(app)
  return (
    <section
      aria-label={t('abuse.title')}
      className="rounded-xl bg-linear-to-b from-card to-muted/50 text-card-foreground shadow-card ring-1 ring-foreground/10"
    >
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 pt-4 pb-3">
        <ShieldCheckGlassIcon
          className="size-5 text-tone-success"
          aria-hidden="true"
        />
        <h3 className="text-sm font-medium">{t('abuse.title')}</h3>
        {data ? (
          <Badge variant={conditionVariant(data)}>
            {t(`abuse.status.${data.condition.reason}`, {
              defaultValue: data.condition.reason,
            })}
          </Badge>
        ) : null}
        <p className="basis-full text-xs text-muted-foreground">
          {t('abuse.subtitle')}
        </p>
        {data &&
        (data.condition.reason === 'ClientIPHidden' ||
          data.condition.reason === 'NoRoutes') ? (
          <Alert className="basis-full">
            <AlertDescription>
              {t(`abuse.statusHint.${data.condition.reason}`)}
            </AlertDescription>
          </Alert>
        ) : null}
      </div>
      {isPending ? <Skeleton className="mx-4 mb-4 h-24" /> : null}
      {error && !data ? (
        <p className="px-4 pb-4 text-sm text-destructive" role="alert">
          {t('abuse.loadError', { message: error.message })}
        </p>
      ) : null}
      {data ? <CardBody app={app} data={data} counters={counters} /> : null}
    </section>
  )
}
