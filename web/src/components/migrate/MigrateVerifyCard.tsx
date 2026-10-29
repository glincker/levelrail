import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { RotateCwGlassIcon, ShieldCheckGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { ApiError } from '../../lib/apiError'
import {
  useLatestVerify,
  useRunVerify,
  type VerifyDifference,
  type VerifyProbeResult,
  type VerifyReport,
  type VerifySeverity,
  type VerifyVerdict,
} from '../../queries/migrateVerify'
import type { MigratePlatform } from '../../queries/migrateWizard'

const PLATFORMS: { id: MigratePlatform; label: string }[] = [
  { id: 'coolify', label: 'Coolify' },
  { id: 'dokploy', label: 'Dokploy' },
  { id: 'caprover', label: 'CapRover' },
]

const PAGE = 50

const VERDICT_VARIANT: Record<
  VerifyVerdict,
  'success' | 'warning' | 'destructive'
> = { pass: 'success', warn: 'warning', fail: 'destructive' }

const SEVERITY_VARIANT: Record<
  VerifySeverity,
  'destructive' | 'warning' | 'info'
> = { blocking: 'destructive', warning: 'warning', info: 'info' }

function ProbeText({ r }: { r: VerifyProbeResult }) {
  const { t } = useTranslation('migrate')
  if (r.error) return <>{t('parity.probeError', { error: r.error })}</>
  const parts = [`${r.status ?? 0}`, `${r.latency_ms ?? 0} ms`]
  if (r.tls) {
    parts.push(
      r.tls_valid
        ? t('parity.probeTls', { days: r.tls_days_left ?? 0 })
        : t('parity.probeTlsBad'),
    )
  }
  if (r.redirect_target) parts.push(`-> ${r.redirect_target}`)
  return <>{parts.join(' · ')}</>
}

function DifferenceRow({ d }: { d: VerifyDifference }) {
  const { t } = useTranslation('migrate')
  return (
    <li className="flex flex-wrap items-baseline gap-x-3 gap-y-1 py-2">
      <Badge variant={SEVERITY_VARIANT[d.severity]}>
        {t(`parity.severity.${d.severity}`)}
      </Badge>
      <span className="text-xs text-muted-foreground">
        {t(`parity.area.${d.category}`)}
      </span>
      <span className="font-medium break-all">
        {d.app ? `${d.app} ` : ''}
        {d.subject}
      </span>
      <span className="text-muted-foreground">{d.detail}</span>
      <span className="basis-full pl-1 text-xs text-muted-foreground">
        {t('parity.fix')}: {d.fix}
      </span>
    </li>
  )
}

function ReportBody({ report }: { report: VerifyReport }) {
  const { t } = useTranslation('migrate')
  const [limit, setLimit] = useState(PAGE)
  const diffs = report.differences ?? []
  const probes = report.probes ?? []
  return (
    <div className="space-y-3">
      <ul className="flex flex-wrap gap-2">
        {report.categories
          .filter((c) => c.source + c.target > 0 || c.blocking + c.warning > 0)
          .map((c) => (
            <li
              key={c.category}
              className="rounded-lg px-2.5 py-1 text-xs ring-1 ring-foreground/10"
            >
              <span className="font-medium">
                {t(`parity.area.${c.category}`)}
              </span>{' '}
              <span className="text-muted-foreground">
                {t('parity.matched', { matched: c.matched, source: c.source })}
              </span>
              {c.blocking > 0 ? (
                <span className="ml-1 text-tone-danger">{c.blocking}</span>
              ) : null}
              {c.warning > 0 ? (
                <span className="ml-1 text-tone-warning">{c.warning}</span>
              ) : null}
            </li>
          ))}
      </ul>
      {(report.notes ?? []).map((n) => (
        <p key={n} className="text-xs text-muted-foreground">
          {n}
        </p>
      ))}
      <div>
        <p className="text-xs font-medium text-muted-foreground">
          {t('parity.differences')}
        </p>
        {diffs.length === 0 ? (
          <p className="py-2 text-sm text-muted-foreground">
            {t('parity.noDifferences')}
          </p>
        ) : (
          <ul className="divide-y divide-foreground/10 text-sm">
            {diffs.slice(0, limit).map((d, i) => (
              <DifferenceRow
                key={`${d.category}-${d.app}-${d.subject}-${i}`}
                d={d}
              />
            ))}
          </ul>
        )}
        {diffs.length > limit ? (
          <Button
            size="sm"
            variant="ghost"
            onClick={() => setLimit((n) => n + PAGE)}
          >
            {t('parity.showMore')}
          </Button>
        ) : null}
      </div>
      {probes.length > 0 ? (
        <div>
          <p className="text-xs font-medium text-muted-foreground">
            {t('parity.probes')}
          </p>
          <ul className="divide-y divide-foreground/10 text-xs">
            {probes.slice(0, limit).map((p) => (
              <li
                key={`${p.app}-${p.domain}`}
                className="grid gap-x-3 gap-y-1 py-1.5 sm:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)_minmax(0,1.2fr)]"
              >
                <span className="font-medium break-all">{p.domain}</span>
                <span>
                  <span className="text-muted-foreground">
                    {t('parity.probeSource')}:{' '}
                  </span>
                  <ProbeText r={p.source} />
                </span>
                <span>
                  <span className="text-muted-foreground">
                    {t('parity.probeHere')}:{' '}
                  </span>
                  <ProbeText r={p.target} />
                </span>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </div>
  )
}

export function MigrateVerifyCard() {
  const { t } = useTranslation('migrate')
  const latest = useLatestVerify()
  const run = useRunVerify()
  const [platform, setPlatform] = useState<MigratePlatform>('coolify')
  const [url, setUrl] = useState('')
  const [token, setToken] = useState('')
  const [targetIp, setTargetIp] = useState('')
  const [skipProbes, setSkipProbes] = useState(false)
  const [expired, setExpired] = useState(false)

  const report = run.data ?? latest.data ?? null
  const canRecheck = Boolean(report?.can_recheck) && !expired

  function onError(e: unknown) {
    if (e instanceof ApiError && e.status === 409) {
      setExpired(true)
      return
    }
    toast.add({
      title: e instanceof Error ? e.message : t('parity.runError'),
      type: 'error',
    })
  }

  function recheck() {
    run.mutate(
      { recheck: true },
      { onError, onSuccess: () => setExpired(false) },
    )
  }

  function submit(e: FormEvent) {
    e.preventDefault()
    run.mutate(
      {
        platform,
        url: url.trim(),
        token,
        target_ip: targetIp.trim() || undefined,
        skip_probes: skipProbes || undefined,
      },
      {
        onError,
        onSuccess: () => {
          setToken('')
          setExpired(false)
        },
      },
    )
  }

  return (
    <section
      aria-label={t('parity.title')}
      className="space-y-3 rounded-xl bg-linear-to-b from-card to-muted/50 p-4 text-sm shadow-card ring-1 ring-foreground/10"
    >
      <div className="flex flex-wrap items-center gap-2">
        <ShieldCheckGlassIcon className="size-5.5 text-muted-foreground" />
        <h2 className="font-medium">{t('parity.title')}</h2>
        {report ? (
          <>
            <Badge variant={VERDICT_VARIANT[report.verdict]}>
              {t(`parity.verdict.${report.verdict}`)}
            </Badge>
            {(['blocking', 'warning', 'info'] as const).map((s) =>
              report.counts[s] > 0 ? (
                <Badge key={s} variant={SEVERITY_VARIANT[s]}>
                  {t('parity.count', {
                    count: report.counts[s],
                    label: t(`parity.severity.${s}`),
                  })}
                </Badge>
              ) : null,
            )}
            <span className="text-xs text-muted-foreground">
              {t('parity.ranAt', {
                when: new Date(report.generated_at).toLocaleString(),
              })}
            </span>
          </>
        ) : (
          <span className="text-xs text-muted-foreground">
            {t('parity.never')}
          </span>
        )}
        {canRecheck ? (
          <Button
            size="sm"
            variant="outline"
            className="ml-auto"
            disabled={run.isPending}
            onClick={recheck}
          >
            <RotateCwGlassIcon size="1.2em" />
            {run.isPending ? t('parity.running') : t('parity.recheck')}
          </Button>
        ) : null}
      </div>
      <p className="text-xs text-muted-foreground">{t('parity.description')}</p>

      {latest.isPending ? (
        <Skeleton className="h-10 w-full rounded-lg" />
      ) : null}
      {latest.isError ? (
        <Banner variant="critical" title={t('parity.loadError')}>
          {latest.error.message}
        </Banner>
      ) : null}
      {expired ? (
        <Banner variant="warning" title={t('parity.expired')} />
      ) : null}

      {canRecheck ? null : (
        <form
          className="flex flex-wrap items-end gap-2"
          onSubmit={submit}
          aria-label={t('parity.run')}
        >
          <div
            className="flex gap-1"
            role="group"
            aria-label={t('parity.platform')}
          >
            {PLATFORMS.map((p) => (
              <Button
                key={p.id}
                type="button"
                size="sm"
                variant={platform === p.id ? 'default' : 'outline'}
                aria-pressed={platform === p.id}
                onClick={() => setPlatform(p.id)}
              >
                {p.label}
              </Button>
            ))}
          </div>
          <div className="min-w-40 flex-1 space-y-1">
            <Label htmlFor="parity-url">{t('parity.url')}</Label>
            <Input
              id="parity-url"
              required
              value={url}
              placeholder={t('connect.urlPlaceholder')}
              onChange={(e) => setUrl(e.target.value)}
            />
          </div>
          <div className="min-w-32 flex-1 space-y-1">
            <Label htmlFor="parity-token">{t('parity.token')}</Label>
            <Input
              id="parity-token"
              type="password"
              required
              autoComplete="off"
              value={token}
              onChange={(e) => setToken(e.target.value)}
            />
          </div>
          <div className="min-w-32 flex-1 space-y-1">
            <Label htmlFor="parity-ip">{t('parity.targetIp')}</Label>
            <Input
              id="parity-ip"
              value={targetIp}
              title={t('parity.targetIpHint')}
              onChange={(e) => setTargetIp(e.target.value)}
            />
          </div>
          <Label className="flex items-center gap-2 pb-2 text-xs font-normal">
            <Checkbox
              aria-label={t('parity.skipProbes')}
              checked={skipProbes}
              onCheckedChange={(v) => setSkipProbes(v === true)}
            />
            {t('parity.skipProbes')}
          </Label>
          <Button type="submit" size="sm" disabled={run.isPending}>
            {run.isPending
              ? t('parity.running')
              : report
                ? t('parity.recheck')
                : t('parity.run')}
          </Button>
        </form>
      )}

      {run.isError && !expired ? (
        <Banner variant="critical" title={t('parity.runError')}>
          {run.error.message}
        </Banner>
      ) : null}

      {report ? <ReportBody report={report} /> : null}
    </section>
  )
}
