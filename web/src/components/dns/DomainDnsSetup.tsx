import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  ArrowLeftGlassIcon,
  ExternalLinkGlassIcon,
  PencilGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import { domainsQueryOptions } from '@/queries/domains'
import { dnsZonesQueryOptions } from '@/queries/dns'
import {
  applyDomainDns,
  detectDnsProvider,
  fetchMailDnsDomains,
  planDomainDns,
  type DnsApplyResult,
} from '@/queries/domainDns'
import {
  applyMailDns,
  checkMailDns,
  planMailDns,
  type MailCheckReport,
  type MailDnsSpec,
} from '@/queries/mailDns'
import { fetchDnsStatus, progressOf } from '@/queries/dnsStatus'
import { startDomainConnectApply } from '@/queries/domainConnect'
import { useServerAddress } from '@/queries/serverAddress'
import { DnsApplyOptions } from './DnsApplyOptions'
import { DnsConnectPopover } from './DnsConnectPopover'
import { DnsHostTile } from './DnsHostChip'
import { DnsPickStep, type PickCandidate } from './DnsPickStep'
import { DnsRecordTable, StatusIcon } from './DnsRecordTable'
import { DnsSummaryBar, type CopyFormat } from './DnsSummaryBar'
import { useCopy } from './useCopy'
import {
  detectState,
  jsonText,
  primaryActionOf,
  providerLabel,
  rememberDomain,
  rememberedDomain,
  rowStatus,
  tableText,
  zoneFileText,
  type SetupStep,
} from './dnsSetup'
import { useBoundedRecheck } from './useBoundedRecheck'

// Public preset ids from internal/mailrecords/presets.go; the API owns the values.
const PRESETS = [
  ['google', 'Google Workspace'],
  ['fastmail', 'Fastmail'],
  ['proton', 'Proton Mail'],
  ['zoho', 'Zoho Mail'],
  ['ses', 'Amazon SES'],
  ['mailgun', 'Mailgun'],
  ['postmark', 'Postmark'],
] as const

export interface DomainDnsSetupProps {
  kind: 'mail' | 'app'
  /** A fixed domain skips the pick step. */
  domain?: string
  app?: string
  defaults?: { domain?: string; hostname?: string }
  onReport?: (r: MailCheckReport) => void
  onDomain?: (domain: string) => void
  /** Bumped by the next-step button to jump straight to the records. */
  previewToken?: number
  /** Existing manual records view, shown when the DNS host is unknown. */
  fallback?: ReactNode
}

const errText = (e: unknown) => (e instanceof Error ? e.message : '')

function useAgoSeconds(at: number | null): number | null {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 5000)
    return () => window.clearInterval(id)
  }, [])
  return at ? Math.max(0, Math.round((now - at) / 1000)) : null
}

/** One-question DNS wizard: pick, detect, review, verify. Shared by the mail tab and the app Domains page. */
export function DomainDnsSetup({
  kind,
  domain: lockedDomain,
  app,
  defaults,
  onReport,
  onDomain,
  previewToken = 0,
  fallback,
}: DomainDnsSetupProps) {
  const { t } = useTranslation('dnsSetup')
  const qc = useQueryClient()
  const locked = !!lockedDomain
  const steps: SetupStep[] = locked
    ? ['detect', 'review', 'verify']
    : ['pick', 'detect', 'review', 'verify']
  const [step, setStep] = useState<SetupStep>(steps[0] ?? 'pick')
  const [chosen, setChosen] = useState(lockedDomain ?? rememberedDomain(kind))
  const [hostOverride, setHostOverride] = useState<string | null>(null)
  const [ipOverride, setIpOverride] = useState('')
  const [editing, setEditing] = useState(false)
  const [preset, setPreset] = useState('')
  const [showPresets, setShowPresets] = useState(false)
  const [mode, setMode] = useState<'auto' | 'myself'>('auto')
  const [replaceKinds, setReplaceKinds] = useState<string[]>([])
  const [confirmReplace, setConfirmReplace] = useState(false)
  const [applied, setApplied] = useState<DnsApplyResult | null>(null)
  const [applyError, setApplyError] = useState('')
  const [report, setReport] = useState<MailCheckReport | undefined>()
  const [checkedAt, setCheckedAt] = useState<number | null>(null)
  const [checking, setChecking] = useState(false)
  const rootRef = useRef<HTMLElement>(null)
  const { copied, copy } = useCopy()

  const mailDomains = useQuery({
    queryKey: ['dns-setup', 'mail-domains'],
    queryFn: fetchMailDnsDomains,
    enabled: kind === 'mail' && !locked,
  })
  const appDomains = useQuery({
    ...domainsQueryOptions(),
    enabled: kind === 'app' && !locked,
  })
  const zones = useQuery({
    ...dnsZonesQueryOptions(),
    enabled: kind === 'app' && !locked,
  })

  const candidates: PickCandidate[] = (() => {
    if (locked) return []
    if (kind === 'mail') return mailDomains.data?.domains ?? []
    const map = new Map<string, PickCandidate>()
    for (const d of appDomains.data ?? []) {
      if (d.automatic) continue
      map.set(d.domain, { domain: d.domain, sources: ['app'] })
    }
    for (const z of zones.data?.zones ?? []) {
      const hit = map.get(z.name)
      if (hit) hit.sources.push('zone')
      else map.set(z.name, { domain: z.name, sources: ['zone'] })
    }
    return [...map.values()]
  })()

  const domain = (
    chosen ||
    (kind === 'mail' ? defaults?.domain : '') ||
    candidates[0]?.domain ||
    ''
  )
    .trim()
    .toLowerCase()
  useEffect(() => {
    onDomain?.(domain)
  }, [domain, onDomain])

  const pick = (d: string) => {
    setChosen(d)
    setHostOverride(null)
    setReplaceKinds([])
    setApplied(null)
    rememberDomain(kind, d)
  }

  const server = useServerAddress('', kind === 'mail')
  const mailHost =
    hostOverride ??
    (defaults?.hostname && domain === defaults.domain
      ? defaults.hostname
      : domain
        ? `mail.${domain}`
        : '')
  const ip = ipOverride.trim() || server.data?.ipv4 || ''
  const spec: MailDnsSpec = {
    ...(preset
      ? { domain, preset }
      : { domain, mail_host: mailHost, ipv4: ip || undefined }),
    ...(replaceKinds.length > 0 ? { replace_existing: replaceKinds } : {}),
  }

  const detect = useQuery({
    queryKey: ['dns-setup', 'detect', domain],
    queryFn: () => detectDnsProvider(domain),
    enabled: !!domain,
    staleTime: 60_000,
  })
  const det = detect.data
  const state = detectState(det)
  const provider = det?.provider ?? ''
  const label = providerLabel(provider)

  const planQ = useQuery({
    queryKey: [
      'dns-setup',
      'plan',
      kind,
      domain,
      app ?? '',
      JSON.stringify(spec),
    ],
    queryFn: () =>
      kind === 'mail' ? planMailDns(spec) : planDomainDns(domain, app),
    enabled: !!domain && (step === 'review' || step === 'verify'),
    staleTime: 0,
  })
  const plan = planQ.data
  const statusQ = useQuery({
    queryKey: [
      'dns-setup',
      'status',
      kind,
      domain,
      app ?? '',
      spec.preset ?? '',
      spec.mail_host ?? '',
      spec.ipv4 ?? '',
    ],
    queryFn: () => fetchDnsStatus(kind, domain, app, spec),
    enabled: !!domain && (step === 'review' || step === 'verify'),
    staleTime: 0,
  })
  const liveStatus = statusQ.data

  const applyMut = useMutation({
    mutationFn: (hash: string) =>
      kind === 'mail'
        ? applyMailDns(spec, hash)
        : applyDomainDns(domain, hash, app),
    onSuccess: (res) => {
      setApplied(res)
      setApplyError('')
      setConfirmReplace(false)
      setStep('verify')
      toast.add({
        title: t('toast.applied', { count: res.applied }),
        type: 'success',
      })
      void qc.invalidateQueries({ queryKey: ['dns-setup', 'plan'] })
    },
    onError: (e) => setApplyError(errText(e) || t('review.applyFailed')),
  })

  const approveMut = useMutation({
    mutationFn: () => startDomainConnectApply(domain, kind),
    onSuccess: () => {
      setApplyError('')
      setStep('verify')
    },
    onError: () => setApplyError(t('options.failed')),
  })

  // Not connected: public-DNS check results stand in for the zone read.
  const overlay = kind === 'mail' && state !== 'connected' ? report : undefined
  const statuses = plan
    ? plan.changes.map((c) => rowStatus(c, overlay, liveStatus))
    : []
  const { live, total } = progressOf(statuses, liveStatus)
  const allLive = total > 0 && live === total

  const runCheck = async () => {
    setChecking(true)
    try {
      await Promise.all([planQ.refetch(), statusQ.refetch()])
      if (kind === 'mail' && domain) {
        const r = await checkMailDns(spec)
        setReport(r)
        onReport?.(r)
      }
    } catch {
      // a failed lookup leaves rows Unknown; the next attempt retries
    } finally {
      setCheckedAt(Date.now())
      setChecking(false)
    }
  }
  const recheck = useBoundedRecheck({
    active: step === 'verify',
    done: allLive,
    run: runCheck,
  })
  const enteredVerify = useRef(false)
  useEffect(() => {
    if (step !== 'verify') {
      enteredVerify.current = false
      return
    }
    if (enteredVerify.current) return
    enteredVerify.current = true
    void runCheck()
    // runCheck closes over fresh state each render; only the step edge matters
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [step])

  const [seenToken, setSeenToken] = useState(0)
  if (previewToken > 0 && previewToken !== seenToken && domain) {
    setSeenToken(previewToken)
    setStep('review')
  }
  useEffect(() => {
    if (previewToken === 0) return
    const calm = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
    rootRef.current?.scrollIntoView?.({
      block: 'start',
      behavior: calm ? 'auto' : 'smooth',
    })
  }, [previewToken])

  const idx = steps.indexOf(step)
  const back = () => {
    const prev = steps[idx - 1]
    if (prev) {
      setStep(prev)
      setConfirmReplace(false)
    }
  }

  const adds = plan?.changes.filter((c) => c.action === 'add').length ?? 0
  const staged = plan?.changes.filter((c) => c.action === 'replace').length ?? 0
  const diff =
    plan?.changes.filter((c) => c.action === 'refused' && c.before).length ?? 0
  const count = adds + staged
  // An empty hash means the plan was synthesised and cannot be applied.
  const action =
    plan?.hash === '' && state === 'connected'
      ? 'copy'
      : primaryActionOf(state, count, det?.connectable ?? false)
  const blocked = !!plan?.blocked && count === 0

  const onPrimary = () => {
    if (!plan) return
    if (action === 'add') {
      if (staged > 0 && !confirmReplace) setConfirmReplace(true)
      else applyMut.mutate(plan.hash)
    } else if (action === 'copy') {
      copy('all', zoneFileText(plan))
      setStep('verify')
    } else if (action === 'check') {
      setStep('verify')
    }
  }
  const toggleReplace = (k: string) => {
    setConfirmReplace(false)
    setReplaceKinds((cur) =>
      cur.includes(k) ? cur.filter((x) => x !== k) : [...cur, k],
    )
  }

  const ptr = report?.items.find((i) => i.id === 'ptr')
  const ago = useAgoSeconds(checkedAt)
  const hasChecks = !!liveStatus || !!overlay || step === 'verify'
  // Add myself, or a host we cannot write to, turns the primary into Copy.
  const myself = action === 'copy' || (mode === 'myself' && action !== 'check')
  const copyAs = (f: CopyFormat) => {
    if (!plan) return
    const text =
      f === 'zone'
        ? zoneFileText(plan)
        : f === 'table'
          ? tableText(plan)
          : jsonText(plan)
    copy('all', text)
  }
  const runPrimary = () => {
    if (myself) {
      if (plan) copy('all', zoneFileText(plan))
      setStep('verify')
    } else onPrimary()
  }
  const primaryNode =
    action === 'connect' && !myself ? (
      <DnsConnectPopover provider={provider} label={label}>
        {t('connectAction', { provider: label })}
      </DnsConnectPopover>
    ) : (
      <Button
        type="button"
        size="sm"
        autoFocus
        disabled={applyMut.isPending || blocked}
        onClick={runPrimary}
      >
        {applyMut.isPending
          ? t('review.applying')
          : myself
            ? t('review.copy')
            : action === 'add'
              ? confirmReplace
                ? t('review.confirmButton', { count: staged })
                : diff > 0
                  ? t('review.addDiff', { count, diff })
                  : t('review.add', { count })
              : t('review.check')}
      </Button>
    )

  const connectOptions = action === 'connect' && !myself
  const showCards = connectOptions && det?.domain_connect?.flow === true
  const optionsHost = det?.domain_connect?.host ?? label

  return (
    <section
      ref={rootRef}
      id="mail-dns-panel"
      aria-label={t('title')}
      onKeyDown={(e) => {
        if (
          e.key === 'Enter' &&
          (e.metaKey || e.ctrlKey) &&
          step === 'review' &&
          plan &&
          !(action === 'connect' && !myself)
        ) {
          e.preventDefault()
          runPrimary()
          return
        }
        if (e.key === 'Escape' && idx > 0 && !e.defaultPrevented) {
          e.preventDefault()
          back()
        }
      }}
      className="space-y-3 rounded-lg border border-border bg-card p-3"
    >
      <header className="flex items-center gap-2">
        {idx > 0 && (
          <Button
            type="button"
            size="xs"
            variant="ghost"
            className="gap-1"
            onClick={back}
          >
            <ArrowLeftGlassIcon size="1.1em" />
            {t('back')}
          </Button>
        )}
        <h2 className="min-w-0 flex-1 truncate text-sm font-medium">
          {t(`step.${step}`)}
          {domain && step !== 'pick' ? (
            <span className="font-mono text-muted-foreground"> {domain}</span>
          ) : null}
        </h2>
        <ol
          aria-label={t('progress', { current: idx + 1, total: steps.length })}
          className="flex gap-1"
        >
          {steps.map((s, i) => (
            <li
              key={s}
              aria-current={i === idx ? 'step' : undefined}
              className={cn(
                'size-1.5 rounded-full',
                i <= idx ? 'bg-primary' : 'bg-border',
              )}
            />
          ))}
        </ol>
      </header>

      {kind === 'app' && step !== 'pick' && (
        <p className="text-xs font-medium">
          {t('serverAddress.recordsTitle', {
            ns: 'domains',
            defaultValue: t('review.recordsTitle'),
          })}
        </p>
      )}

      {step === 'pick' && (
        <div className="space-y-3">
          {mailDomains.isLoading || appDomains.isLoading ? (
            <div
              role="status"
              aria-label={t('loading')}
              className="h-12 animate-pulse rounded-md bg-muted motion-reduce:animate-none"
            />
          ) : mailDomains.isError ? (
            <p role="alert" className="text-xs text-tone-danger">
              {t('pick.loadFailed')}
            </p>
          ) : null}
          <DnsPickStep
            kind={kind}
            candidates={candidates}
            selected={domain}
            detectedProvider={provider}
            onSelect={(d) => {
              pick(d)
            }}
            onUseSuggestion={(apex, host) => {
              pick(apex)
              setHostOverride(host)
            }}
          />
          <Button
            type="button"
            size="sm"
            disabled={!domain}
            autoFocus
            onClick={() => setStep('detect')}
          >
            {t('continue')}
          </Button>
        </div>
      )}

      {step === 'detect' && (
        <div className="space-y-3">
          {detect.isLoading ? (
            <div
              role="status"
              aria-label={t('detect.detecting')}
              className="flex items-center gap-3"
            >
              <div className="size-8 animate-pulse rounded-md bg-muted motion-reduce:animate-none" />
              <div className="h-4 w-48 animate-pulse rounded bg-muted motion-reduce:animate-none" />
            </div>
          ) : detect.isError ? (
            <p role="alert" className="text-sm text-tone-danger">
              {t('detect.failed')}
            </p>
          ) : (
            <div className="flex items-center gap-3">
              <DnsHostTile
                provider={state === 'unknown' ? '' : provider}
                size="md"
              />
              <p className="text-sm" data-testid="detect-sentence">
                {state === 'connected' &&
                  t('detect.connected', { provider: label })}
                {state === 'notConnected' &&
                  t('detect.notConnected', { provider: label })}
                {state === 'unknown' && t('detect.unknown')}
              </p>
            </div>
          )}
          {det?.domain_connect?.supported && (
            <p className="text-xs text-muted-foreground">
              {t('detect.domainConnect', {
                host: det.domain_connect?.host ?? label,
              })}
            </p>
          )}
          <div className="flex flex-wrap items-center gap-2">
            {state === 'notConnected' && det?.connectable ? (
              <>
                <DnsConnectPopover provider={provider} label={label}>
                  {t('connectAction', { provider: label })}
                </DnsConnectPopover>
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  onClick={() => setStep('review')}
                >
                  {t('detect.without')}
                </Button>
              </>
            ) : (
              <Button
                type="button"
                size="sm"
                autoFocus
                disabled={detect.isLoading}
                onClick={() => setStep('review')}
              >
                {t('continue')}
              </Button>
            )}
          </div>
        </div>
      )}

      {step === 'review' && (
        <div className="space-y-2">
          <p
            className="flex items-center gap-1.5 text-xs text-muted-foreground"
            data-testid="detect-banner"
          >
            <DnsHostTile
              provider={state === 'unknown' ? '' : provider}
              size="xs"
            />
            <span>
              {state === 'unknown'
                ? t('banner.unknown')
                : t('banner.hosted', { provider: label })}
            </span>
            {state !== 'unknown' && <span>{`· ${t(`banner.${state}`)}`}</span>}
            {state === 'notConnected' && det?.connectable && (
              <DnsConnectPopover
                provider={provider}
                label={label}
                variant="ghost"
                size="xs"
              >
                {t('options.connectLink')}
              </DnsConnectPopover>
            )}
          </p>
          {planQ.isLoading && (
            <div
              role="status"
              aria-label={t('loading')}
              className="h-24 animate-pulse rounded-md bg-muted motion-reduce:animate-none"
            />
          )}
          {planQ.isError && (
            <p role="alert" className="text-xs text-tone-danger">
              {t('review.planFailed', { message: errText(planQ.error) })}
            </p>
          )}
          {plan && plan.changes.length === 0 && (
            <p className="text-xs text-muted-foreground">{t('review.empty')}</p>
          )}
          {plan && plan.changes.length > 0 && (
            <>
              <DnsSummaryBar
                count={count}
                live={live}
                total={total}
                checked={hasChecks}
                primary={showCards ? null : primaryNode}
                onCopy={copyAs}
              />
              {plan.blocked && (
                <p role="alert" className="text-xs text-tone-danger">
                  {t('review.blocked')}
                </p>
              )}
              {showCards && (
                <DnsApplyOptions
                  host={optionsHost}
                  flow
                  approving={approveMut.isPending}
                  certs={det?.can_issue_certificates ?? false}
                  mail={kind === 'mail'}
                  onApprove={() => approveMut.mutate()}
                  token={
                    <DnsConnectPopover
                      provider={provider}
                      label={label}
                      variant="outline"
                    >
                      {t('options.connectToken', { host: optionsHost })}
                    </DnsConnectPopover>
                  }
                />
              )}
              <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5">
                {(action === 'add' || action === 'connect') && (
                  <div
                    role="group"
                    aria-label={t('mode.label')}
                    className="inline-flex overflow-hidden rounded-md border border-border text-xs"
                  >
                    {(['auto', 'myself'] as const).map((m) => (
                      <button
                        key={m}
                        type="button"
                        aria-pressed={mode === m}
                        onClick={() => setMode(m)}
                        className={cn(
                          'px-2.5 py-1 focus-visible:outline-2 focus-visible:outline-ring',
                          m === 'myself' && 'border-l border-border',
                          mode === m
                            ? 'bg-muted font-medium'
                            : 'hover:bg-muted',
                        )}
                      >
                        {t(`mode.${m}`)}
                      </button>
                    ))}
                  </div>
                )}
                {myself ? (
                  <p className="flex items-center gap-2 text-xs text-muted-foreground">
                    {state === 'unknown'
                      ? t('manual.whereUnknown')
                      : t('manual.whereKnown', { provider: label })}
                    {det?.dashboard_url && (
                      <a
                        href={det.dashboard_url}
                        target="_blank"
                        rel="noreferrer"
                        className="inline-flex items-center gap-1 underline underline-offset-2"
                      >
                        {t('openIn', { provider: label })}
                        <ExternalLinkGlassIcon size="1.1em" />
                      </a>
                    )}
                  </p>
                ) : action === 'connect' && !showCards ? (
                  <span className="text-xs text-muted-foreground">
                    {t('options.tokenHint')}
                    {det?.can_issue_certificates && kind === 'mail'
                      ? ` ${t('options.tokenCerts')}`
                      : ''}
                  </span>
                ) : (
                  action === 'add' && (
                    <span className="text-xs text-muted-foreground">
                      {t('review.nothingDeleted')}
                    </span>
                  )
                )}
                {copied === 'all' && (
                  <span
                    className="text-xs text-muted-foreground"
                    aria-live="polite"
                  >
                    {t('copied')}
                  </span>
                )}
              </div>
              <DnsRecordTable
                kind={kind}
                plan={plan}
                report={overlay}
                status={liveStatus}
                phase="review"
                replaceKinds={replaceKinds}
                onToggleReplace={toggleReplace}
                checkedAgo={ago}
              />
              {state === 'unknown' && fallback}
              {confirmReplace && (
                <p
                  role="alert"
                  className="rounded-md border border-border bg-muted p-2 text-xs"
                >
                  {t('review.confirmReplace', { count: staged })}
                </p>
              )}
              {applyError && (
                <p role="alert" className="text-xs text-tone-danger">
                  {applyError}
                </p>
              )}
            </>
          )}
          {kind === 'mail' && !preset && (
            <div className="flex flex-wrap items-center gap-2 text-sm">
              {editing || !ip ? (
                <>
                  <div className="space-y-1">
                    <Label htmlFor="dns-mail-host" className="text-xs">
                      {t('review.host')}
                    </Label>
                    <Input
                      id="dns-mail-host"
                      className="h-8 w-48"
                      value={mailHost}
                      onChange={(e) => setHostOverride(e.target.value)}
                    />
                  </div>
                  <div className="space-y-1">
                    <Label htmlFor="dns-mail-ip" className="text-xs">
                      {t('review.ip')}
                    </Label>
                    <Input
                      id="dns-mail-ip"
                      className="h-8 w-36"
                      inputMode="decimal"
                      value={ipOverride || ip}
                      placeholder={t('review.ipPlaceholder')}
                      onChange={(e) => setIpOverride(e.target.value)}
                    />
                  </div>
                  {ip && (
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() => setEditing(false)}
                    >
                      {t('review.done')}
                    </Button>
                  )}
                </>
              ) : (
                <>
                  <p
                    className="min-w-0 break-all font-mono text-xs"
                    data-testid="host-line"
                  >
                    {mailHost} {'->'} {ip}
                  </p>
                  <Button
                    type="button"
                    size="xs"
                    variant="ghost"
                    aria-label={t('review.edit')}
                    onClick={() => setEditing(true)}
                  >
                    <PencilGlassIcon size="1.1em" />
                  </Button>
                </>
              )}
            </div>
          )}
          {kind === 'mail' && (
            <div className="text-xs">
              <button
                type="button"
                className="underline underline-offset-2"
                aria-expanded={showPresets}
                onClick={() => setShowPresets((v) => !v)}
              >
                {t('review.otherProvider')}
              </button>
              {showPresets && (
                <div className="mt-1.5 max-w-60">
                  <Select
                    value={preset || 'self'}
                    onValueChange={(v) => {
                      setPreset(v && v !== 'self' ? v : '')
                      setReplaceKinds([])
                    }}
                  >
                    <SelectTrigger
                      aria-label={t('review.presetLabel')}
                      className="w-full"
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="self">
                        {t('review.selfHosted')}
                      </SelectItem>
                      {PRESETS.map(([id, name]) => (
                        <SelectItem key={id} value={id}>
                          {name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              )}
            </div>
          )}
        </div>
      )}

      {step === 'verify' && (
        <div className="space-y-2">
          {applied && (
            <p
              className="text-xs text-muted-foreground"
              data-testid="apply-summary"
            >
              {t('verify.summary', {
                added:
                  applied.records?.filter((r) => r.action === 'add').length ??
                  applied.applied,
                same:
                  applied.records?.filter((r) => r.action !== 'add').length ??
                  0,
              })}
            </p>
          )}
          <DnsSummaryBar
            count={statuses.length - live}
            live={live}
            total={statuses.length}
            checked
            primary={null}
            onCopy={copyAs}
            checking={checking}
            onCheckAgain={() => {
              recheck.restart()
              void runCheck()
            }}
          />
          {plan && (
            <div className={cn(checking && 'motion-safe:animate-pulse')}>
              <DnsRecordTable
                kind={kind}
                plan={plan}
                report={overlay}
                status={liveStatus}
                phase="verify"
                checkedAgo={ago}
              />
            </div>
          )}
          {kind === 'mail' && (
            <p
              className="flex items-start gap-1.5 text-xs text-muted-foreground"
              data-testid="ptr-line"
            >
              <StatusIcon
                status={ptr?.status === 'pass' ? 'live' : 'waiting'}
              />
              <span>{ptr?.message || t('verify.ptr')}</span>
            </p>
          )}
          <p className="text-xs text-muted-foreground" aria-live="polite">
            {allLive
              ? t('verify.allLive')
              : recheck.exhausted
                ? t('verify.stillWaiting')
                : t('verify.waiting')}
            {ago != null && ` · ${t('verify.checkedAgo', { seconds: ago })}`}
          </p>
        </div>
      )}
    </section>
  )
}
